package main

import (
	"context"
	"fmt"
	"os"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/knovate211/proto/codec"
	userv1 "github.com/knovate211/proto/user/v1"
)

func main() {
	if len(os.Args) < 5 {
		fmt.Println("Error: Missing arguments.")
		fmt.Println("Usage: go run add-user.go <email> <name> <password> <role>")
		fmt.Println("Example: go run add-user.go prabhatkonly@gmail.com \"Prabhat\" Hello@123 student")
		os.Exit(1)
	}

	email := os.Args[1]
	name := os.Args[2]
	password := os.Args[3]
	role := os.Args[4]

	// Register codec so it knows how to marshal/unmarshal
	codec.Register()

	addr := os.Getenv("USER_SERVICE_ADDR")
	if addr == "" {
		addr = "localhost:50055"
	}

	ctx := context.Background()

	conn, err := grpc.Dial(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		fmt.Printf("Error: Unable to connect to user-service at %s: %v\n", addr, err)
		os.Exit(1)
	}
	defer conn.Close()

	client := userv1.NewUserServiceClient(conn)

	_, err = client.CreateOrUpdateUser(ctx, &userv1.CreateOrUpdateUserRequest{
		Email:    email,
		Name:     name,
		Password: password,
		Role:     role,
	})
	if err != nil {
		fmt.Printf("Error: Failed to insert/update user: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("\nSuccessfully added/updated user via user-service:\n")
	fmt.Printf("  Name:     %s\n", name)
	fmt.Printf("  Email:    %s\n", email)
	fmt.Printf("  Role:     %s\n", role)
}
