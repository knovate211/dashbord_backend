// Package sandbox runs user code inside ephemeral Docker containers.
package sandbox

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"go.uber.org/zap"
)

// Language-to-Docker-image mapping for the language runners.
var languageImages = map[string]string{
	"python":     "skillofide/runner-python:latest",
	"javascript": "skillofide/runner-javascript:latest",
	"java":       "skillofide/runner-java:latest",
	"cpp":        "skillofide/runner-cpp:latest",
	"go":         "skillofide/runner-go:latest",
	"sql":        "skillofide/runner-sql:latest",
}

// RunRequest contains everything needed to execute user code against one test case.
type RunRequest struct {
	ProblemId     string
	Language      string
	Code          string
	Input         string
	TimeLimitMs   int32
	MemoryLimitMb int32
	// Spec is the problem's declared execution contract. When it is present and
	// its IoMode is "function", the submission is wrapped by the generated
	// driver for its signature. When it is absent — a problem not yet migrated
	// onto a signature — wrapping falls back to the legacy slug-keyed drivers.
	Spec *ExecutionSpec

	// Raw runs the submitted code verbatim, skipping the driver that is
	// normally generated around a solution function. Scratchpad code is a
	// complete program with its own entry point, so wrapping it would break it.
	Raw bool
}

// RunResult is the output from executing one test case in the sandbox.
type RunResult struct {
	Stdout      string
	Stderr      string
	ExitCode    int64
	ExecutionMs int64
	MemoryKb    int64
	TimedOut    bool
	OOMKilled   bool
}

// DockerSandbox executes code inside Docker containers with resource limits.
type DockerSandbox struct {
	cli *client.Client
	log *zap.Logger
	// slots bounds how many containers run at once. See New.
	slots chan struct{}
}

// New creates a DockerSandbox using the DOCKER_HOST environment variable (or default socket).
//
// Concurrency is bounded on purpose. Every container is given a full CPU, so
// letting an unbounded number start together does not make the judge faster —
// it makes each one slower, and once they are slow enough they blow their time
// limit and come back as TimeLimitExceeded. A candidate cannot tell that from a
// wrong answer, so under load the judge silently starts marking correct code
// wrong. A dozen simultaneous runs on a laptop took 21 seconds each and every
// one failed that way.
//
// Queueing is the better failure: waiting for a slot costs a candidate a few
// seconds and still returns the right verdict. The wait is deliberately outside
// the timed region in Run, so queue time is never charged to their code.
func New(log *zap.Logger) (*DockerSandbox, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, fmt.Errorf("create docker client: %w", err)
	}

	n := maxConcurrentRuns()
	log.Info("docker sandbox ready", zap.Int("max_concurrent_runs", n))
	return &DockerSandbox{cli: cli, log: log, slots: make(chan struct{}, n)}, nil
}

// Capacity is how many containers may run at once.
func (s *DockerSandbox) Capacity() int { return cap(s.slots) }

// maxConcurrentRuns sizes the pool. Each container is pinned to one CPU, so the
// question is how many can run before they start stealing time from each other
// and from the services around them.
//
// Half the cores, measured rather than guessed. On an 8-core host, eight
// simultaneous runs of the same accepted solution came back:
//
//	2 →  8/8 accepted, p50 4.8s
//	3 →  8/8 accepted, p50 3.8s
//	4 →  8/8 accepted, p50 6.3s
//	5 →  8/8 accepted, p50 6.9s
//	6 →  6/8 TIMED OUT, p50 37s     ← correct code, marked wrong
//
// The cliff is sharp and it is silent: past it the judge does not slow down
// gracefully, it starts failing correct submissions. NumCPU/2 sits well clear
// of it with room for a busier host, and EXEC_MAX_CONCURRENT tunes it for
// hardware that measures differently.
func maxConcurrentRuns() int {
	if v := os.Getenv("EXEC_MAX_CONCURRENT"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	n := runtime.NumCPU() / 2
	if n < 2 {
		n = 2
	}
	return n
}

// startupGraceMs is the allowance added to every language's time limit to cover
// container start and interpreter boot. Tunable because the right value depends
// entirely on the host: near-zero on a dedicated Linux runner, seconds on a
// laptop VM or a box under drive-day load.
func startupGraceMs() int {
	if v := os.Getenv("EXEC_STARTUP_GRACE_MS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			return n
		}
	}
	return 3000
}

// pidsLimit caps processes and threads in one run. It is generous because
// threads count too: the JVM, the Go compiler and the SQL runner's Postgres
// each start dozens. EXEC_PIDS_LIMIT overrides it.
func pidsLimit() int64 {
	if v := os.Getenv("EXEC_PIDS_LIMIT"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil && n > 0 {
			return n
		}
	}
	return 256
}

// maxOutputBytes caps how much of each stream is kept. A print loop can emit
// hundreds of megabytes before its time limit; without a cap it is all held in
// memory here and then shipped through NATS.
const maxOutputBytes = 1 << 20

// Run executes code for a single test case and returns the raw output.
func (s *DockerSandbox) Run(ctx context.Context, req *RunRequest) (*RunResult, error) {
	image, ok := languageImages[req.Language]
	if !ok {
		return nil, fmt.Errorf("unsupported language: %s", req.Language)
	}

	// Wait for a slot before anything is timed. A queued run is a slow run, not
	// a failed one.
	if s.slots != nil {
		select {
		case s.slots <- struct{}{}:
			defer func() { <-s.slots }()
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	timeLimitMs := req.TimeLimitMs
	if timeLimitMs <= 0 {
		timeLimitMs = 2000
	}
	// The clock below starts at ContainerStart, so it covers the container
	// coming up and the interpreter booting as well as the candidate's code.
	// On a Linux host that overhead is a couple of hundred milliseconds and
	// nobody notices; on a busy host — a campus drive with a dozen runs landing
	// together — it is seconds, and a correct solution comes back as
	// TimeLimitExceeded, which a candidate cannot tell from a wrong answer.
	//
	// So every language gets a startup allowance on top of its problem time
	// limit. It buys the container's boot, not the algorithm's runtime: a real
	// infinite loop is still killed, just a moment later.
	timeLimitMs += int32(startupGraceMs())

	// Compiled and heavyweight languages need more on top, for the compiler or
	// the VM rather than for the container.
	langLower := strings.ToLower(req.Language)
	if langLower == "cpp" || langLower == "go" {
		timeLimitMs += 8000
	} else if langLower == "java" {
		// Java needs extra time: javac compilation (3-5s) + JVM startup (2-3s) in Alpine Docker
		timeLimitMs += 15000
	} else if langLower == "sql" {
		// The SQL runner boots a PostgreSQL cluster and builds the fixture
		// tables before the query runs. The cluster is pre-initialised at image
		// build time, so this only covers pg_ctl start plus schema setup.
		timeLimitMs += 10000
	}
	memLimitMb := req.MemoryLimitMb
	if memLimitMb <= 0 {
		memLimitMb = 256
	}

	// Wrap user code with driver code to read input and print output.
	// Raw requests are complete programs and must not be wrapped.
	wrappedCode := req.Code
	if !req.Raw {
		var err error
		wrappedCode, err = wrapForSpec(req.Spec, req.ProblemId, req.Language, req.Code)
		if err != nil {
			return nil, err
		}
	}

	// Build environment variables to pass code + input into the container
	envVars := []string{
		"USER_CODE=" + wrappedCode,
		"USER_INPUT=" + req.Input,
	}

	// Create container config
	containerCfg := &container.Config{
		Image: image,
		Env:   envVars,
	}
	// Create container with strict resource limits. The code is untrusted and
	// the sandbox talks to the host's Docker daemon, so the container gets no
	// network, no capabilities, no way to gain privileges, no swap, and a cap
	// on processes so a fork bomb stays inside it.
	memBytes := int64(memLimitMb) * 1024 * 1024
	pids := pidsLimit()
	resp, err := s.cli.ContainerCreate(ctx,
		containerCfg,
		&container.HostConfig{
			NetworkMode: "none", // no network access for security
			CapDrop:     []string{"ALL"},
			SecurityOpt: []string{"no-new-privileges"},
			Resources: container.Resources{
				Memory:     memBytes,
				MemorySwap: memBytes,      // equal to Memory: no swap
				NanoCPUs:   1_000_000_000, // 1 CPU core
				PidsLimit:  &pids,
			},
		},
		nil, nil, "",
	)
	if err != nil {
		return nil, fmt.Errorf("create container: %w", err)
	}

	containerID := resp.ID
	defer func() {
		// Always clean up
		rmCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		s.cli.ContainerRemove(rmCtx, containerID, container.RemoveOptions{Force: true}) //nolint:errcheck
	}()

	start := time.Now()

	if err := s.cli.ContainerStart(ctx, containerID, container.StartOptions{}); err != nil {
		return nil, fmt.Errorf("start container: %w", err)
	}

	// Wait with a strict time limit
	timeoutCtx, timeoutCancel := context.WithTimeout(ctx, time.Duration(timeLimitMs)*time.Millisecond+500*time.Millisecond)
	defer timeoutCancel()

	statusCh, errCh := s.cli.ContainerWait(timeoutCtx, containerID, container.WaitConditionNotRunning)

	var exitCode int64
	var timedOut bool
	var oomKilled bool

	select {
	case waitResp := <-statusCh:
		exitCode = waitResp.StatusCode
		if waitResp.Error != nil {
			s.log.Warn("container wait error", zap.String("err", waitResp.Error.Message))
		}
	case err := <-errCh:
		if err != nil {
			timedOut = true
			s.cli.ContainerKill(context.Background(), containerID, "SIGKILL") //nolint:errcheck
		}
	case <-timeoutCtx.Done():
		timedOut = true
		s.cli.ContainerKill(context.Background(), containerID, "SIGKILL") //nolint:errcheck
	}

	executionMs := time.Since(start).Milliseconds()

	// Collect inspect info for memory stats
	inspect, inspectErr := s.cli.ContainerInspect(ctx, containerID)
	if inspectErr == nil && inspect.State != nil {
		oomKilled = inspect.State.OOMKilled
	}

	// Collect stdout/stderr.
	// Docker's ContainerLogs returns a multiplexed stream. Each frame has an
	// 8-byte header: [stream(1)] [pad(3)] [size(4 big-endian)], then payload.
	// We demultiplex it manually since stdcopy is not vendored.
	logReader, err := s.cli.ContainerLogs(ctx, containerID, container.LogsOptions{
		ShowStdout: true,
		ShowStderr: true,
	})
	var stdout, stderr string
	if err == nil {
		defer logReader.Close()
		var stdoutBuf, stderrBuf bytes.Buffer
		hdr := make([]byte, 8)
		for {
			_, err := io.ReadFull(logReader, hdr)
			if err != nil {
				break
			}
			frameSize := binary.BigEndian.Uint32(hdr[4:8])
			if frameSize == 0 {
				continue
			}
			var buf *bytes.Buffer
			switch hdr[0] {
			case 1: // stdout
				buf = &stdoutBuf
			case 2: // stderr
				buf = &stderrBuf
			}
			// Keep up to the cap and discard the rest without allocating it.
			keep := int64(0)
			if buf != nil {
				keep = min(int64(frameSize), int64(maxOutputBytes-buf.Len()))
			}
			if keep > 0 {
				if _, err := io.CopyN(buf, logReader, keep); err != nil {
					break
				}
			}
			if _, err := io.CopyN(io.Discard, logReader, int64(frameSize)-keep); err != nil {
				break
			}
		}
		stdout = stdoutBuf.String()
		stderr = stderrBuf.String()
	}

	return &RunResult{
		Stdout:      stdout,
		Stderr:      stderr,
		ExitCode:    exitCode,
		ExecutionMs: executionMs,
		TimedOut:    timedOut,
		OOMKilled:   oomKilled,
	}, nil
}

func wrapUserCode(problemId string, language string, code string) string {
	language = strings.ToLower(language)
	problemId = strings.ToLower(problemId)

	// Map database UUIDs to slugs for custom driver wrappers
	switch problemId {
	case "54574a34-9a68-4e65-ab9a-af05db4c0001":
		problemId = "op1"
	case "54574a34-9a68-4e65-ab9a-af05db4c0002":
		problemId = "arr5"
	case "54574a34-9a68-4e65-ab9a-af05db4c0003":
		problemId = "cond1"
	case "54574a34-9a68-4e65-ab9a-af05db4c0004":
		problemId = "loop1"
	case "54574a34-9a68-4e65-ab9a-af05db4c0005":
		problemId = "str2"
	}

	// SQL submissions are handed to the runner verbatim. There is no driver to
	// wrap them in: the harness inside the container builds the fixture tables,
	// executes this statement, and serialises the result rows itself.
	if language == "sql" {
		return code
	}

	switch language {
	case "javascript":
		var driver string
		switch problemId {
		case "op1":
			driver = `
const fs = require('fs');
const input = fs.readFileSync('/dev/stdin', 'utf-8').trim().split('\n');
const a = parseInt(input[0]);
const b = parseInt(input[1]);
console.log(JSON.stringify(arithmeticOperations(a, b)));
`
		case "arr5":
			driver = `
const fs = require('fs');
const input = fs.readFileSync('/dev/stdin', 'utf-8').trim().split('\n');
const nums = JSON.parse(input[0]);
const target = parseInt(input[1]);
console.log(JSON.stringify(twoSum(nums, target)));
`
		case "cond1":
			driver = `
const fs = require('fs');
const n = parseInt(fs.readFileSync('/dev/stdin', 'utf-8').trim());
console.log(JSON.stringify(checkEvenOdd(n)));
`
		case "loop1":
			driver = `
const fs = require('fs');
const n = parseInt(fs.readFileSync('/dev/stdin', 'utf-8').trim());
console.log(sumOfN(n));
`
		case "str2":
			driver = `
const fs = require('fs');
const s = JSON.parse(fs.readFileSync('/dev/stdin', 'utf-8').trim());
console.log(isPalindrome(s));
`
		default:
			driver = `
const fs = require('fs');
const input = fs.readFileSync('/dev/stdin', 'utf-8').trim();
try {
    const code = fs.readFileSync(__filename, 'utf-8');
    const match = code.match(/function\s+([a-zA-Z0-9_]+)\s*\(/);
    const funcName = match ? match[1] : 'solveChallenge';
    let args;
    try {
        args = JSON.parse("[" + input + "]");
    } catch(e) {
        args = [input];
    }
    const func = eval(funcName);
    const result = func.apply(null, args);
    if (result !== undefined) {
        console.log(JSON.stringify(result));
    } else if (args.length > 0 && typeof args[0] === 'object') {
        console.log(JSON.stringify(args[0]));
    }
} catch (e) {
    console.error(e);
}
`
		}
		return code + "\n" + driver

	case "python":
		var driver string
		switch problemId {
		case "op1":
			driver = `
import sys
input_data = sys.stdin.read().splitlines()
a = int(input_data[0])
b = int(input_data[1])
print(arithmeticOperations(a, b))
`
		case "arr5":
			driver = `
import sys, json
input_data = sys.stdin.read().splitlines()
nums = json.loads(input_data[0])
target = int(input_data[1])
print(json.dumps(twoSum(nums, target)))
`
		case "cond1":
			driver = `
import sys, json
n = int(sys.stdin.read().trim())
print(json.dumps(checkEvenOdd(n)))
`
		case "loop1":
			driver = `
import sys
n = int(sys.stdin.read().trim())
print(sumOfN(n))
`
		case "str2":
			driver = `
import sys, json
s = json.loads(sys.stdin.read().trim())
print(str(isPalindrome(s)).lower())
`
		default:
			driver = `
import sys, json, re
input_data = sys.stdin.read().strip()
try:
    try:
        args = json.loads("[" + input_data + "]")
    except:
        args = [input_data]
    with open(__file__, "r", encoding="utf-8") as f:
        code = f.read()
    match = re.search(r'def\s+([a-zA-Z0-9_]+)\s*\(', code)
    func_name = match.group(1) if match else 'solveChallenge'
    if func_name in globals():
        func = globals()[func_name]
        result = func(*args)
        if result is not None:
            print(json.dumps(result))
        elif len(args) > 0 and isinstance(args[0], (list, dict)):
            print(json.dumps(args[0]))
except Exception as e:
    print(e, file=sys.stderr)
`
		}
		return code + "\n" + driver

	case "java":
		var javaMain string
		switch problemId {
		case "op1":
			javaMain = `
    public static void main(String[] args) throws Exception {
        java.io.BufferedReader br = new java.io.BufferedReader(new java.io.InputStreamReader(System.in));
        String line1 = br.readLine();
        String line2 = br.readLine();
        if (line1 == null || line2 == null) return;
        int a = Integer.parseInt(line1.trim());
        int b = Integer.parseInt(line2.trim());
        Solution sol = new Solution();
        int[] res = sol.arithmeticOperations(a, b);
        System.out.println(java.util.Arrays.toString(res).replace(" ", ""));
    }
`
		case "arr5":
			javaMain = `
    public static void main(String[] args) throws Exception {
        java.io.BufferedReader br = new java.io.BufferedReader(new java.io.InputStreamReader(System.in));
        String arrayStr = br.readLine();
        String targetStr = br.readLine();
        if (arrayStr == null || targetStr == null) return;
        arrayStr = arrayStr.trim();
        int target = Integer.parseInt(targetStr.trim());
        arrayStr = arrayStr.substring(1, arrayStr.length() - 1);
        String[] tokens = arrayStr.split(",");
        int[] nums = new int[tokens.length];
        for (int i = 0; i < tokens.length; i++) {
            nums[i] = Integer.parseInt(tokens[i].trim());
        }
        Solution sol = new Solution();
        int[] res = sol.twoSum(nums, target);
        System.out.println(java.util.Arrays.toString(res).replace(" ", ""));
    }
`
		case "cond1":
			javaMain = `
    public static void main(String[] args) throws Exception {
        java.io.BufferedReader br = new java.io.BufferedReader(new java.io.InputStreamReader(System.in));
        String line = br.readLine();
        if (line == null) return;
        int n = Integer.parseInt(line.trim());
        Solution sol = new Solution();
        String res = sol.checkEvenOdd(n);
        System.out.println("\"" + res + "\"");
    }
`
		case "loop1":
			javaMain = `
    public static void main(String[] args) throws Exception {
        java.io.BufferedReader br = new java.io.BufferedReader(new java.io.InputStreamReader(System.in));
        String line = br.readLine();
        if (line == null) return;
        int n = Integer.parseInt(line.trim());
        Solution sol = new Solution();
        System.out.println(sol.sumOfN(n));
    }
`
		case "str2":
			javaMain = `
    public static void main(String[] args) throws Exception {
        java.io.BufferedReader br = new java.io.BufferedReader(new java.io.InputStreamReader(System.in));
        String s = br.readLine();
        if (s == null) return;
        s = s.trim();
        if (s.startsWith("\"") && s.endsWith("\"")) {
            s = s.substring(1, s.length() - 1);
        }
        Solution sol = new Solution();
        System.out.println(sol.isPalindrome(s));
    }
`
		default:
			javaMain = `
    public static void main(String[] args) throws Exception {
        java.io.BufferedReader br = new java.io.BufferedReader(new java.io.InputStreamReader(System.in));
        String input = br.readLine();
        if (input == null) return;
        input = input.trim();
        Solution sol = new Solution();
        java.lang.reflect.Method targetMethod = null;
        for (java.lang.reflect.Method m : Solution.class.getDeclaredMethods()) {
            if (!m.getName().equals("main") && !m.getName().equals("solveChallenge")) {
                targetMethod = m;
                break;
            }
        }
        if (targetMethod == null) {
            // Standalone void main class provided by the student (e.g. hello world)
            return;
        }
        Class<?>[] paramTypes = targetMethod.getParameterTypes();
        Object[] parsedArgs = new Object[paramTypes.length];
        String[] parts;
        if (paramTypes.length == 1) {
            parts = new String[]{input};
        } else {
            java.util.List<String> list = new java.util.ArrayList<>();
            int bracketCount = 0;
            boolean inQuotes = false;
            StringBuilder sb = new StringBuilder();
            for (int i = 0; i < input.length(); i++) {
                char c = input.charAt(i);
                if (c == '"' && (i == 0 || input.charAt(i-1) != '\\')) inQuotes = !inQuotes;
                if (!inQuotes) {
                    if (c == '[') bracketCount++;
                    else if (c == ']') bracketCount--;
                }
                if (c == ',' && bracketCount == 0 && !inQuotes) {
                    list.add(sb.toString().trim());
                    sb = new StringBuilder();
                } else {
                    sb.append(c);
                }
            }
            list.add(sb.toString().trim());
            parts = list.toArray(new String[0]);
        }
        for (int i = 0; i < paramTypes.length; i++) {
            parsedArgs[i] = parseJavaValue(parts[i], paramTypes[i]);
        }
        Object result = targetMethod.invoke(sol, parsedArgs);
        if (targetMethod.getReturnType() == void.class) {
            printJavaValue(parsedArgs[0]);
        } else {
            printJavaValue(result);
        }
    }
    private static Object parseJavaValue(String val, Class<?> type) throws Exception {
        val = val.trim();
        if (type == int.class || type == Integer.class) {
            return Integer.parseInt(val);
        } else if (type == long.class || type == Long.class) {
            return Long.parseLong(val);
        } else if (type == double.class || type == Double.class) {
            return Double.parseDouble(val);
        } else if (type == boolean.class || type == Boolean.class) {
            return Boolean.parseBoolean(val);
        } else if (type == String.class) {
            if (val.startsWith("\"") && val.endsWith("\"")) {
                val = val.substring(1, val.length() - 1);
            }
            return val;
        } else if (type == int[].class) {
            if (val.startsWith("[")) val = val.substring(1);
            if (val.endsWith("]")) val = val.substring(0, val.length() - 1);
            if (val.trim().isEmpty()) return new int[0];
            String[] tokens = val.split(",");
            int[] arr = new int[tokens.length];
            for (int i = 0; i < tokens.length; i++) arr[i] = Integer.parseInt(tokens[i].trim());
            return arr;
        } else if (type == String[].class) {
            if (val.startsWith("[")) val = val.substring(1);
            if (val.endsWith("]")) val = val.substring(0, val.length() - 1);
            if (val.trim().isEmpty()) return new String[0];
            String[] tokens = val.split(",");
            for (int i = 0; i < tokens.length; i++) {
                tokens[i] = tokens[i].trim();
                if (tokens[i].startsWith("\"") && tokens[i].endsWith("\"")) {
                    tokens[i] = tokens[i].substring(1, tokens[i].length() - 1);
                }
            }
            return tokens;
        }
        return null;
    }
    private static void printJavaValue(Object val) {
        if (val == null) {
            System.out.println("null");
        } else if (val instanceof int[]) {
            System.out.println(java.util.Arrays.toString((int[]) val).replace(" ", ""));
        } else if (val instanceof Object[]) {
            System.out.println(java.util.Arrays.deepToString((Object[]) val).replace(" ", ""));
        } else if (val instanceof String) {
            System.out.println("\"" + val + "\"");
        } else {
            System.out.println(val.toString());
        }
    }
`
		}
		// Inject inside public class Solution
		normalizedCode := strings.ReplaceAll(code, " ", "")
		normalizedCode = strings.ReplaceAll(normalizedCode, "\t", "")
		normalizedCode = strings.ReplaceAll(normalizedCode, "\r", "")
		normalizedCode = strings.ReplaceAll(normalizedCode, "\n", "")
		if strings.Contains(normalizedCode, "voidmain(") {
			return code
		}
		lastBrace := strings.LastIndex(code, "}")
		if lastBrace != -1 {
			return code[:lastBrace] + "\n" + javaMain + "\n}"
		}
		return code

	case "cpp":
		var driver string
		switch problemId {
		case "op1":
			driver = `
#include <iostream>
int main() {
    int a, b;
    if (std::cin >> a >> b) {
        Solution sol;
        std::vector<int> res = sol.arithmeticOperations(a, b);
        std::cout << "[";
        for (size_t i = 0; i < res.size(); ++i) {
            std::cout << res[i] << (i == res.size() - 1 ? "" : ",");
        }
        std::cout << "]" << std::endl;
    }
    return 0;
}
`
		case "arr5":
			driver = `
#include <iostream>
#include <string>
#include <sstream>
int main() {
    std::string arrayStr;
    int target;
    if (std::getline(std::cin, arrayStr) && std::cin >> target) {
        if (arrayStr.front() == '[') arrayStr = arrayStr.substr(1);
        if (arrayStr.back() == ']') arrayStr.pop_back();
        std::vector<int> nums;
        std::stringstream ss(arrayStr);
        std::string item;
        while (std::getline(ss, item, ',')) {
            nums.push_back(std::stoi(item));
        }
        Solution sol;
        std::vector<int> res = sol.twoSum(nums, target);
        std::cout << "[";
        for (size_t i = 0; i < res.size(); ++i) {
            std::cout << res[i] << (i == res.size() - 1 ? "" : ",");
        }
        std::cout << "]" << std::endl;
    }
    return 0;
}
`
		case "cond1":
			driver = `
#include <iostream>
int main() {
    int n;
    if (std::cin >> n) {
        Solution sol;
        std::cout << "\"" << sol.checkEvenOdd(n) << "\"" << std::endl;
    }
    return 0;
}
`
		case "loop1":
			driver = `
#include <iostream>
int main() {
    int n;
    if (std::cin >> n) {
        Solution sol;
        std::cout << sol.sumOfN(n) << std::endl;
    }
    return 0;
}
`
		case "str2":
			driver = `
#include <iostream>
#include <string>
int main() {
    std::string s;
    if (std::getline(std::cin, s)) {
        if (s.front() == '"' && s.back() == '"') {
            s = s.substr(1, s.length() - 2);
        }
        Solution sol;
        std::cout << (sol.isPalindrome(s) ? "true" : "false") << std::endl;
    }
    return 0;
}
`
		default:
			funcName := "solveChallenge"
			retType := "void"
			hasParams := false

			re := regexp.MustCompile(`(std::)?(vector<[^>]+>|string|int|long\s+long|double|bool|void)\s+([a-zA-Z0-9_]+)\s*\(([^)]*)\)`)
			matches := re.FindStringSubmatch(code)
			if len(matches) >= 5 {
				retType = matches[2]
				funcName = matches[3]
				paramsStr := matches[4]
				if strings.TrimSpace(paramsStr) != "" {
					hasParams = true
				}
			}

			var paramParsers []string
			var paramNames []string
			if hasParams {
				paramSpecs := strings.Split(matches[4], ",")
				for idx, spec := range paramSpecs {
					spec = strings.TrimSpace(spec)
					parts := strings.Fields(spec)
					if len(parts) >= 2 {
						pType := strings.Join(parts[:len(parts)-1], " ")
						pName := fmt.Sprintf("arg%d", idx)
						paramNames = append(paramNames, pName)

						if pType == "int" {
							paramParsers = append(paramParsers, fmt.Sprintf("int %s; ss >> %s; ss.ignore(1);", pName, pName))
						} else if pType == "long long" {
							paramParsers = append(paramParsers, fmt.Sprintf("long long %s; ss >> %s; ss.ignore(1);", pName, pName))
						} else if pType == "double" {
							paramParsers = append(paramParsers, fmt.Sprintf("double %s; ss >> %s; ss.ignore(1);", pName, pName))
						} else if pType == "bool" {
							paramParsers = append(paramParsers, fmt.Sprintf("bool %s; ss >> std::boolalpha >> %s; ss.ignore(1);", pName, pName))
						} else if pType == "string" || pType == "std::string" {
							paramParsers = append(paramParsers, fmt.Sprintf("std::string %s; std::getline(ss, %s, ',');", pName, pName))
						} else if strings.Contains(pType, "vector<int>") {
							paramParsers = append(paramParsers, fmt.Sprintf(`
        std::string vecStr%d;
        std::getline(ss, vecStr%d, ']');
        if (vecStr%d.front() == '[') vecStr%d = vecStr%d.substr(1);
        std::vector<int> %s;
        std::stringstream vecSS%d(vecStr%d);
        std::string item%d;
        while (std::getline(vecSS%d, item%d, ',')) {
            if (!item%d.empty()) %s.push_back(std::stoi(item%d));
        }
        ss.ignore(1);
`, idx, idx, idx, idx, idx, pName, idx, idx, idx, idx, idx, idx, pName, idx))
						} else {
							paramParsers = append(paramParsers, fmt.Sprintf("std::string %s; ss >> %s;", pName, pName))
						}
					}
				}
			}

			callStr := fmt.Sprintf("sol.%s(%s)", funcName, strings.Join(paramNames, ", "))
			printStr := ""
			if retType == "void" {
				if len(paramNames) > 0 && strings.Contains(matches[4], "vector") {
					printStr = fmt.Sprintf(`
        %s;
        std::cout << "[";
        for (size_t i = 0; i < %s.size(); ++i) {
            std::cout << %s[i] << (i == %s.size() - 1 ? "" : ",");
        }
        std::cout << "]" << std::endl;
`, callStr, paramNames[0], paramNames[0], paramNames[0])
				} else {
					printStr = callStr + ";"
				}
			} else if strings.Contains(retType, "vector<int>") {
				printStr = fmt.Sprintf(`
        std::vector<int> res = %s;
        std::cout << "[";
        for (size_t i = 0; i < res.size(); ++i) {
            std::cout << res[i] << (i == res.size() - 1 ? "" : ",");
        }
        std::cout << "]" << std::endl;
`, callStr)
			} else if retType == "string" || retType == "std::string" {
				printStr = fmt.Sprintf("std::cout << \"\\\"\" << %s << \"\\\"\" << std::endl;", callStr)
			} else {
				printStr = fmt.Sprintf("std::cout << %s << std::endl;", callStr)
			}

			driver = fmt.Sprintf(`
#include <iostream>
#include <string>
#include <sstream>
#include <vector>
int main() {
    std::string s;
    if (std::getline(std::cin, s)) {
        std::stringstream ss(s);
        %s
        Solution sol;
        %s
    }
    return 0;
}
`, strings.Join(paramParsers, "\n        "), printStr)
		}
		// Bypass custom driver injection if the student provides their own main method
		normalizedCpp := strings.ReplaceAll(code, " ", "")
		normalizedCpp = strings.ReplaceAll(normalizedCpp, "\t", "")
		normalizedCpp = strings.ReplaceAll(normalizedCpp, "\r", "")
		normalizedCpp = strings.ReplaceAll(normalizedCpp, "\n", "")
		if strings.Contains(normalizedCpp, "intmain(") {
			return code
		}
		return code + "\n" + driver

	case "go":
		// A submission that already declares func main() is a complete stdio
		// program — the whole `go-*` family of basics problems is written that
		// way. Appending a generated main() to it produced a second main and a
		// duplicated import block, so every one of them failed to compile no
		// matter what the learner wrote. run.sh pipes USER_INPUT to stdin, so
		// these run correctly verbatim. Mirrors the same guard on Java below.
		if goDeclaresMain(code) {
			return code
		}

		funcName := "solveChallenge"
		retType := ""
		hasParams := false

		re := regexp.MustCompile(`func\s+([a-zA-Z0-9_]+)\s*\(([^)]*)\)\s*([a-zA-Z0-9_*&<>\[\]\s]+)?`)
		matches := re.FindStringSubmatch(code)
		if len(matches) >= 3 {
			funcName = matches[1]
			paramsStr := matches[2]
			if strings.TrimSpace(paramsStr) != "" {
				hasParams = true
			}
			if len(matches) >= 4 {
				retType = strings.TrimSpace(matches[3])
			}
		}

		var paramParsers []string
		var paramNames []string
		if hasParams {
			paramSpecs := strings.Split(matches[2], ",")
			for idx, spec := range paramSpecs {
				spec = strings.TrimSpace(spec)
				parts := strings.Fields(spec)
				if len(parts) >= 2 {
					pName := parts[0]
					pType := parts[1]
					paramNames = append(paramNames, pName)

					if pType == "int" {
						paramParsers = append(paramParsers, fmt.Sprintf("var %s int; fmt.Fscanf(reader, \"%%d\", &%s)", pName, pName))
					} else if pType == "string" {
						paramParsers = append(paramParsers, fmt.Sprintf("var %s string; fmt.Fscanf(reader, \"%%q\", &%s)", pName, pName))
					} else if pType == "[]int" {
						paramParsers = append(paramParsers, fmt.Sprintf(`
	var arrStr%d string
	fmt.Fscan(reader, &arrStr%d)
	arrStr%d = strings.Trim(arrStr%d, "[]")
	var %s []int
	if len(arrStr%d) > 0 {
		for _, item := range strings.Split(arrStr%d, ",") {
			val, _ := strconv.Atoi(strings.TrimSpace(item))
			%s = append(%s, val)
		}
	}
`, idx, idx, idx, idx, pName, idx, idx, pName, pName))
					} else {
						paramParsers = append(paramParsers, fmt.Sprintf("var %s string; fmt.Fscan(reader, &%s)", pName, pName))
					}
				}
			}
		}

		callStr := fmt.Sprintf("%s(%s)", funcName, strings.Join(paramNames, ", "))
		printStr := ""
		if retType == "" {
			if len(paramNames) > 0 && strings.Contains(matches[2], "[]") {
				printStr = fmt.Sprintf(`
	%s
	fmt.Println(strings.ReplaceAll(fmt.Sprintf("%%v", %s), " ", ","))
`, callStr, paramNames[0])
			} else {
				printStr = callStr
			}
		} else if strings.Contains(retType, "[]") {
			printStr = fmt.Sprintf(`
	res := %s
	fmt.Println(strings.ReplaceAll(fmt.Sprintf("%%v", res), " ", ","))
`, callStr)
		} else {
			printStr = fmt.Sprintf("fmt.Println(%s)", callStr)
		}

		goMain := fmt.Sprintf(`
func main() {
	reader := strings.NewReader(os.Getenv("USER_INPUT"))
	%s
	%s
}
`, strings.Join(paramParsers, "\n\t"), printStr)

		code = strings.Replace(code, "package main", "package main\n\nimport (\n\t\"fmt\"\n\t\"strings\"\n\t\"strconv\"\n\t\"os\"\n)", 1)
		return code + "\n" + goMain
	}

	return code
}

// goDeclaresMain reports whether the submission already provides func main().
// Comments and strings are not stripped first: a false positive costs a
// learner nothing (their program is run as-is, which is what a complete
// program wants), while a false negative reintroduces the duplicate-main
// compile error this guards against.
var goMainRe = regexp.MustCompile(`(?m)^\s*func\s+main\s*\(\s*\)`)

func goDeclaresMain(code string) bool {
	return goMainRe.MatchString(code)
}
