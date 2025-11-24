package setup

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/catalystcommunity/foundry/v1/internal/config"
	"github.com/catalystcommunity/foundry/v1/internal/setup"
)

// StepExecutor defines the interface for executing a setup step
type StepExecutor interface {
	Execute(ctx context.Context, cfg *config.Config) error
	Validate(ctx context.Context, cfg *config.Config) error
	Description() string
}

// Wizard manages the interactive setup process
type Wizard struct {
	configPath string
	config     *config.Config
	state      *setup.SetupState
	executors  map[setup.Step]StepExecutor
}

// NewWizard creates a new setup wizard
func NewWizard(configPath string) (*Wizard, error) {
	// Load config
	cfg, err := config.Load(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}

	// Load setup state
	state, err := setup.LoadState(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load setup state: %w", err)
	}

	w := &Wizard{
		configPath: configPath,
		config:     cfg,
		state:      state,
		executors:  make(map[setup.Step]StepExecutor),
	}

	// Register step executors (will be implemented as we build components)
	w.registerExecutors()

	return w, nil
}

// registerExecutors registers all step executors
func (w *Wizard) registerExecutors() {
	// TODO: Register actual executors as we implement components
	// w.executors[setup.StepNetworkPlan] = &NetworkPlanExecutor{}
	// w.executors[setup.StepNetworkValidate] = &NetworkValidateExecutor{}
	// etc.
}

// Run executes the setup wizard
func (w *Wizard) Run(ctx context.Context) error {
	// Set up signal handling for graceful shutdown
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Create a done channel to signal goroutine to exit
	done := make(chan struct{})
	defer close(done)

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sigChan) // Clean up signal handler

	go func() {
		select {
		case <-sigChan:
			fmt.Println("\n\nSetup interrupted. Progress has been saved.")
			cancel()
		case <-ctx.Done():
			// Context cancelled, exit goroutine
			return
		case <-done:
			// Function is returning, exit goroutine
			return
		}
	}()

	// Check if setup is already complete
	if w.state.IsComplete() {
		fmt.Println("✓ Setup is already complete!")
		return nil
	}

	// Show welcome message
	w.showWelcome()

	// Show current progress
	w.showProgress()

	// Determine where to resume from
	nextStep := setup.DetermineNextStep(w.state)
	if nextStep != setup.StepNetworkPlan {
		fmt.Printf("\nResuming from: %s\n\n", w.stepName(nextStep))
	}

	// Execute steps until complete or interrupted
	for {
		select {
		case <-ctx.Done():
			// Save state before exiting
			if err := setup.SaveState(w.configPath, w.state); err != nil {
				return fmt.Errorf("failed to save state: %w", err)
			}
			return ctx.Err()
		default:
			// Execute next step
			currentStep := setup.DetermineNextStep(w.state)
			if currentStep == setup.StepComplete {
				w.showCompletion()
				return nil
			}

			if err := w.executeStep(ctx, currentStep); err != nil {
				return fmt.Errorf("step %s failed: %w", currentStep, err)
			}

			// Update state after successful step
			if err := w.updateState(currentStep); err != nil {
				return fmt.Errorf("failed to update state: %w", err)
			}

			// Save state after each step
			if err := setup.SaveState(w.configPath, w.state); err != nil {
				return fmt.Errorf("failed to save state: %w", err)
			}

			// Show progress after each step
			w.showProgress()
		}
	}
}

// executeStep executes a single setup step
func (w *Wizard) executeStep(ctx context.Context, step setup.Step) error {
	fmt.Printf("\n▶ %s\n", w.stepName(step))

	// Get executor for this step
	executor, ok := w.executors[step]
	if !ok {
		// If no executor is registered yet, this is a placeholder
		fmt.Printf("  ⚠ Step not yet implemented\n")
		return nil
	}

	// Validate step prerequisites
	fmt.Printf("  Validating prerequisites...\n")
	if err := executor.Validate(ctx, w.config); err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}

	// Execute step
	fmt.Printf("  %s\n", executor.Description())
	if err := executor.Execute(ctx, w.config); err != nil {
		return err
	}

	fmt.Printf("  ✓ Complete\n")
	return nil
}

// updateState updates the setup state after a successful step
func (w *Wizard) updateState(step setup.Step) error {
	switch step {
	case setup.StepNetworkPlan:
		w.state.NetworkPlanned = true
	case setup.StepNetworkValidate:
		w.state.NetworkValidated = true
	case setup.StepOpenBAOInstall:
		w.state.OpenBAOInstalled = true
	case setup.StepDNSInstall:
		w.state.DNSInstalled = true
	case setup.StepDNSZonesCreate:
		w.state.DNSZonesCreated = true
	case setup.StepZotInstall:
		w.state.ZotInstalled = true
	case setup.StepK8sInstall:
		w.state.K8sInstalled = true
	case setup.StepComplete:
		w.state.StackComplete = true
	default:
		return fmt.Errorf("unknown step: %s", step)
	}
	return nil
}

// showWelcome displays the welcome message
func (w *Wizard) showWelcome() {
	fmt.Println("╔════════════════════════════════════════════════════════════╗")
	fmt.Println("║                                                            ║")
	fmt.Println("║              Foundry Stack Setup Wizard                    ║")
	fmt.Println("║                                                            ║")
	fmt.Println("║  This wizard will guide you through setting up your       ║")
	fmt.Println("║  complete Catalyst Community tech stack.                  ║")
	fmt.Println("║                                                            ║")
	fmt.Println("║  Progress is saved at each step. You can safely           ║")
	fmt.Println("║  interrupt and resume at any time.                        ║")
	fmt.Println("║                                                            ║")
	fmt.Println("╚════════════════════════════════════════════════════════════╝")
	fmt.Println()
}

// showProgress displays the current setup progress
func (w *Wizard) showProgress() {
	fmt.Println("\n┌─ Setup Progress ─────────────────────────────────────────┐")
	fmt.Println("│                                                          │")

	steps := []struct {
		name      string
		completed bool
	}{
		{"Network Planning", w.state.NetworkPlanned},
		{"Network Validation", w.state.NetworkValidated},
		{"OpenBAO Installation", w.state.OpenBAOInstalled},
		{"DNS Installation", w.state.DNSInstalled},
		{"DNS Zones Creation", w.state.DNSZonesCreated},
		{"Zot Registry Installation", w.state.ZotInstalled},
		{"Kubernetes Cluster Setup", w.state.K8sInstalled},
		{"Stack Finalization", w.state.StackComplete},
	}

	for _, step := range steps {
		if step.completed {
			fmt.Printf("│  ✓ %-52s │\n", step.name)
		} else {
			fmt.Printf("│  ○ %-52s │\n", step.name)
		}
	}

	fmt.Println("│                                                          │")
	fmt.Println("└──────────────────────────────────────────────────────────┘")
	fmt.Println()
}

// showCompletion displays the completion message
func (w *Wizard) showCompletion() {
	fmt.Println("\n╔════════════════════════════════════════════════════════════╗")
	fmt.Println("║                                                            ║")
	fmt.Println("║              ✓ Setup Complete!                             ║")
	fmt.Println("║                                                            ║")
	fmt.Println("║  Your Foundry stack is ready to use.                      ║")
	fmt.Println("║                                                            ║")
	fmt.Println("║  Next steps:                                               ║")
	fmt.Println("║    • Verify stack status: foundry stack status            ║")
	fmt.Println("║    • List cluster nodes: foundry cluster node list        ║")
	fmt.Println("║    • View DNS zones: foundry dns zone list                ║")
	fmt.Println("║                                                            ║")
	fmt.Println("╚════════════════════════════════════════════════════════════╝")
	fmt.Println()
}

// stepName returns a human-readable name for a step
func (w *Wizard) stepName(step setup.Step) string {
	switch step {
	case setup.StepNetworkPlan:
		return "Network Planning"
	case setup.StepNetworkValidate:
		return "Network Validation"
	case setup.StepOpenBAOInstall:
		return "OpenBAO Installation"
	case setup.StepDNSInstall:
		return "DNS (PowerDNS) Installation"
	case setup.StepDNSZonesCreate:
		return "DNS Zones Creation"
	case setup.StepZotInstall:
		return "Zot Registry Installation"
	case setup.StepK8sInstall:
		return "Kubernetes Cluster Setup"
	case setup.StepComplete:
		return "Setup Complete"
	default:
		return string(step)
	}
}

// Reset resets the setup state (for --reset flag)
func (w *Wizard) Reset() error {
	w.state.Reset()
	if err := setup.SaveState(w.configPath, w.state); err != nil {
		return fmt.Errorf("failed to save reset state: %w", err)
	}
	fmt.Println("Setup state has been reset.")
	return nil
}
