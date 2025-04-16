// main.go
package main

import (
	"context"
	"fmt"
	"github.com/openai/openai-go"
	"github.com/shaharia-lab/goai"
	"os"
	"time"

	"github.com/shaharia-lab/goai/agent"
)

func main() {
	// Create OpenAI LLM Provider
	llmProvider := goai.NewOpenAILLMProvider(goai.OpenAIProviderConfig{
		Client: goai.NewOpenAIClient(os.Getenv("OPENAI_API_KEY")),
		Model:  openai.ChatModelGPT3_5Turbo,
	})

	// Configure LLM Request
	llm := goai.NewLLMRequest(goai.NewRequestConfig(
		goai.WithMaxToken(100),
		goai.WithTemperature(0.7),
	), llmProvider)

	// Initialize the agent
	ai, err := agent.NewAgent(llm, "./data/agent_state")
	if err != nil {
		fmt.Printf("Failed to initialize agent: %v\n", err)
		os.Exit(1)
	}

	// Define a task with multiple steps
	dataAnalysisTask := &agent.Task{
		ID:          "test-task-001",
		Name:        "Bangladesh",
		Description: "Write a story",
		Steps: map[string]*agent.Step{
			"step1": {
				ID:              "step1",
				Description:     "Country exploration",
				Instructions:    "What's the capital of Bangladesh?",
				TimeoutDuration: 2 * time.Minute,
			},
			"step2": {
				ID:              "step2",
				Description:     "Capital of Russia",
				Instructions:    "What is the capital of Russia?",
				TimeoutDuration: 2 * time.Minute,
				DependsOn:       []string{"step1"},
			},
			"step3": {
				ID:              "step3",
				Description:     "tell me a story",
				Instructions:    "tell me a story about Bangladesh",
				TimeoutDuration: 3 * time.Minute,
				DependsOn:       []string{"step2"},
			},
		},
		StepOrder: []string{"step1", "step2", "step3"},
	}

	// Start the task
	ctx := context.Background()
	taskID, err := ai.StartTask(ctx, dataAnalysisTask)
	if err != nil {
		fmt.Printf("Failed to start task: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Started task with ID: %s\n", taskID)

	// Run task steps incrementally
	for {
		fmt.Println("Running next step...")
		complete, err := ai.RunTaskStep(ctx, taskID)
		if err != nil {
			fmt.Printf("Error running task step: %v\n", err)
			break
		}

		// Get current state to show progress
		state, _ := ai.GetTaskState(taskID)
		fmt.Printf("Completed step: %s\n", state.CurrentStepID)

		if complete {
			fmt.Println("Task completed!")
			fmt.Println(state.FinalOutput)
			break
		}

		// In a real application, you might want to wait for a user prompt or a timer
		// before continuing to the next step
		fmt.Println("Press Enter to continue to next step...")
		fmt.Scanln()
	}
}
