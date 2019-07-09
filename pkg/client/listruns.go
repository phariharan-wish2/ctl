package client

import (
	"errors"
	"fmt"
	"github.com/ContextLogic/ctl/pkg/client/helper"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"os"
	"strings"
	"sync"
)

func (c *Client) ListRuns(context string, namespace string, options ListOptions) ([]RunDiscovery, error) {
	cs, err := c.getContextInterface(context)
	if err != nil {
		return nil, err
	}
	runs, err := cs.BatchV1().Jobs(namespace).List(metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	items := make([]RunDiscovery, len(runs.Items))
	for i, run := range runs.Items {
		items[i] = RunDiscovery{context, run}
	}
	return items, nil
}

func (c *Client) ListRunsOverContexts(contexts []string, namespace string, options ListOptions) ([]RunDiscovery, error) {
	if len(contexts) == 0 {
		contexts = helper.GetContexts()
	}

	var wait sync.WaitGroup
	wait.Add(len(contexts))

	var mutex sync.Mutex
	var ret []RunDiscovery
	var failed []string

	for _, ctx := range contexts {
		go func(ctx string) {
			defer wait.Done()

			runs, err := c.ListRuns(ctx, namespace, options)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Could not connect to cluster \"%s\": %v\n", ctx, err)
				failed = append(failed, ctx)
				return
			}

			mutex.Lock()
			for _, run := range runs {
				ret = append(ret, run)
			}
			mutex.Unlock()
		}(ctx)
	}

	wait.Wait()
	if failed != nil {
		return ret, errors.New("Failed connecting to the following contexts: " + strings.Join(failed, ", "))
	}
	return ret, nil
}

// Also over contexts
func (c *Client) ListRunsOfCronJob(contexts []string, namespace, cronjobName string, options ListOptions) ([]RunDiscovery, error) {
	cronjob, err := c.findCronJob(contexts, namespace, cronjobName)
	if err != nil {
		return nil, err
	}

	// Assuming that jobs started are in the same location
	list, err := c.ListRuns(cronjob.Context, cronjob.Namespace, options)
	if err != nil {
		return nil, err
	}

	var ret []RunDiscovery
	for _, r := range list {
		// Check if has owner reference
		for _, o := range r.OwnerReferences {
			if o.UID == cronjob.UID { // matches
				ret = append(ret, r)
				break
			}
		}
	}

	return ret, nil
}
