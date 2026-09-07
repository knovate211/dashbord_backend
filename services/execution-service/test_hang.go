package main

import (
	"context"
	"fmt"

	"github.com/knovate211/execution-service/internal/sandbox"
	"go.uber.org/zap"
)

func main() {
	log, _ := zap.NewDevelopment()
	sb, err := sandbox.New(log)
	if err != nil {
		panic(err)
	}

	code := `import java.util.HashMap;

class Solution {
    public int[] twoSum(int[] nums, int target) {
        HashMap<Integer, Integer> map = new HashMap<>();

        for (int i = 0; i < nums.length; i++) {
            int complement = target - nums[i];

            if (map.containsKey(complement)) {
                return new int[] { map.get(complement), i };
            }

            map.put(nums[i], i);
        }

        return new int[0];
    }
}`

	req := &sandbox.RunRequest{
		ProblemId:     "54574a34-9a68-4e65-ab9a-af05db4c0002",
		Language:      "java",
		Code:          code,
		Input:         "[2,7,11,15]\r\n9\r\n",
		TimeLimitMs:   2000,
		MemoryLimitMb: 256,
	}

	res, err := sb.Run(context.Background(), req)
	if err != nil {
		panic(err)
	}

	fmt.Printf("ExecutionMs: %d\n", res.ExecutionMs)
	fmt.Printf("ExitCode: %d\n", res.ExitCode)
	fmt.Printf("TimedOut: %v\n", res.TimedOut)
	fmt.Printf("Stdout: %q\n", res.Stdout)
	fmt.Printf("Stderr: %q\n", res.Stderr)
}
