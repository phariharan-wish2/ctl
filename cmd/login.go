package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"
	"github.com/wish/ctl/cmd/util/config"
	"github.com/wish/ctl/cmd/util/parsing"
	"github.com/wish/ctl/pkg/client"
	v1 "k8s.io/api/core/v1"
)

const (
	// DefaultLoginCommand is what's used if no loginCommand is found in ctl-config
	DefaultLoginCommand string = "/bin/bash"
)

func loginCmd(c *client.Client) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "login APPNAME [flags]",
		Short: "Uses kubectl exec to run a command to ad hoc pod. Command is defined in ctl-config.",
		Long: `Uses kubectl exec to run a command to ad hoc pod. Command is defined in ctl-config.
If no command is found from the config, it will default to /bin/bash.
Note that this command only operates on one pod, if multiple pods have the exact name,
the command will only work on the first one found.
If the pod has only one container, the container name is optional.
If the pod has multiple containers, it will choose the first container found.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctxs, _ := cmd.Flags().GetStringSlice("context")
			namespace, _ := cmd.Flags().GetString("namespace")
			container, _ := cmd.Flags().GetString("container")
			user, _ := cmd.Flags().GetString("user")

			appName := args[0]

			// Get hostname to use in job name if not supplied
			if user == "" {
				var err error
				user, err = os.Hostname()
				if err != nil {
					return errors.New("Unable to get hostname of machine")
				}
			}

			// We get the pod through the name label
			podName := fmt.Sprintf("%s-%s", appName, user)
			lm, _ := parsing.LabelMatch(fmt.Sprintf("name=%s", podName))
			options := client.ListOptions{LabelMatch: lm}

			pods, err := c.ListPodsOverContexts(ctxs, namespace, options)
			if err != nil {
				return err
			}
			if len(pods) < 1 {
				return fmt.Errorf("No pod found, try running `ctl up %s` to start your pod", appName)
			}

			pod := pods[0]

			podPhase := pod.Status.Phase
			// Check to see if pod is running
			if podPhase == v1.PodPending {
				return fmt.Errorf("Pod %s is still being created", pod.Name)
			}

			// Get the login command from the ctl-config configmap
			m, err := config.GetCtlExt()
			if err != nil {
				return err
			}

			// Get loginCommand to use with kubectl exec from the config file
			loginCommand := []string{}
			if rawruns, ok := m[pod.Context]["_run"]; ok {
				runs := make(map[string]runDetails)
				err := json.Unmarshal([]byte(rawruns), &runs)
				if err != nil {
					return fmt.Errorf("Failed to get rawruns from ctl-config: %v", err)
				}
				loginCommand = runs[appName].LoginCommand
			}
			// If no loginCommand is supplied then default to bash
			if len(loginCommand) < 1 {
				fmt.Printf("Using default command: %v\n", DefaultLoginCommand)
				loginCommand = []string{DefaultLoginCommand}
			}

			fmt.Printf("Running following commands in pod: %s\n"+
				"Use `ctl cp in %s <files> -o <destination>` to copy files into pod\n"+
				"Use `ctl cp out %s <files> -o <destination>` to copy files out of pod\n"+
				"Use `ctl cp -h` for more info about file copying\n\n",
				strings.Join(loginCommand, " "), pod.Name, pod.Name)

			// Build kubectl exec command
			context := fmt.Sprintf("--context=%s", pod.Context)
			namespace = fmt.Sprintf("--namespace=%s", pod.Namespace)
			name := pod.Name
			if container == "" { // If container flag is empty, grab first one
				container = fmt.Sprintf("--container=%s", pod.Spec.Containers[0].Name)
			}

			combinedArgs := append(
				[]string{"exec", "-i", "-t", name, container, context, namespace, "--"},
				loginCommand...,
			)
			command := exec.Command("kubectl", combinedArgs...)
			command.Stdout = os.Stdout
			command.Stderr = os.Stderr
			command.Stdin = os.Stdin

			return command.Run()
		},
	}

	cmd.Flags().StringP("container", "c", "", "Specify the container")
	cmd.Flags().StringP("user", "u", "", "Name that is used for ad hoc jobs. Defaulted to hostname.")

	return cmd
}