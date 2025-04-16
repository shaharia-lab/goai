// main.go
package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/shaharia-lab/goai/agent"
)

func main() {
	// Initialize the LLM provider
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		fmt.Println("Please set the OPENAI_API_KEY environment variable")
		os.Exit(1)
	}

	llmProvider := agent.NewOpenAIProvider(apiKey)

	// Initialize the agent
	ai, err := agent.NewAgent(llmProvider, "./data/agent_state")
	if err != nil {
		fmt.Printf("Failed to initialize agent: %v\n", err)
		os.Exit(1)
	}

	// Define a task with multiple steps
	dataAnalysisTask := &agent.Task{
		ID:          "data-analysis-123",
		Name:        "Data Analysis Report",
		Description: "Analyze the given dataset and create a comprehensive report",
		Steps: map[string]*agent.Step{
			"step1": {
				ID:              "step1",
				Description:     "Data Exploration",
				Instructions:    "Explore the dataset and identify key patterns and statistics.",
				TimeoutDuration: 2 * time.Minute,
			},
			"step2": {
				ID:              "step2",
				Description:     "Data Visualization Planning",
				Instructions:    "Based on the exploration, recommend visualizations that would effectively communicate the findings.",
				TimeoutDuration: 2 * time.Minute,
				DependsOn:       []string{"step1"},
			},
			"step3": {
				ID:              "step3",
				Description:     "Final Report Compilation",
				Instructions:    "Compile a final report with key insights and visualization recommendations.",
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
			// Print final output
			fmt.Println(state.FinalOutput)
			break
		}

		// In a real application, you might want to wait for a user prompt or a timer
		// before continuing to the next step
		fmt.Println("Press Enter to continue to next step...")
		fmt.Scanln()
	}
}
