package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
)

func main() {
	if len(os.Args) < 2 {
		log.Fatalf("Usage: %s <stack-name-or-id>", os.Args[0])
	}
	stackID := os.Args[1]

	ctx := context.Background()

	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		log.Fatalf("failed to load AWS config: %v", err)
	}

	fmt.Printf("Using AWS Region: %s\n", cfg.Region)
	fmt.Printf("Deleting Stack: %s\n", stackID)

	client := cloudformation.NewFromConfig(cfg)

	_, err = client.DeleteStack(ctx, &cloudformation.DeleteStackInput{
		StackName: &stackID,
	})
	if err != nil {
		log.Fatalf("failed to delete stack: %v", err)
	}

	fmt.Println("Stack deletion initiated, waiting for completion...")

	waiter := cloudformation.NewStackDeleteCompleteWaiter(client)
	err = waiter.Wait(ctx, &cloudformation.DescribeStacksInput{
		StackName: &stackID,
	}, 10*time.Minute)
	if err != nil {
		log.Fatalf("failed waiting for stack deletion: %v", err)
	}

	fmt.Println("Stack deleted successfully")
}
