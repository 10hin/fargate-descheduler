package cmd

import (
	"fmt"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/10hin/fargate-descheduler/pkg/server"
)

var (
	rootCmd = &cobra.Command{
		Use:   "fargate-descheduler",
		Short: "fargate-descheduler",
		Long:  `The fargate-descheduler removes pods which violating inter-pod constraints`,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			cs, err := buildClientset(cmd)
			if err != nil {
				return err
			}
			return server.Serve(ctx, cs)
		},
	}
	kubeconfigFlag string
)

func init() {
	rootCmd.PersistentFlags().StringVar(&kubeconfigFlag, "kubeconfig", "", "(optional) Use specified kubeconfig file (path must be absolute; default: use in-cluster config)")
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func buildClientset(cmd *cobra.Command) (*kubernetes.Clientset, error) {
	if kubeconfigFlag != "" {
		clientset, err := buildClientsetWithPath(kubeconfigFlag)
		if err == nil {
			return clientset, nil
		}
	}

	var err error
	var config *rest.Config
	config, err = rest.InClusterConfig()
	if err == nil {
		var clientset *kubernetes.Clientset
		clientset, err = kubernetes.NewForConfig(config)
		if err == nil {
			return clientset, nil
		}
	}

	// fallback to default kubeconfig path
	// respect to KUBECONFIG environment variable

	var kubeconfigPath string
	if kubeconfigEnv := os.Getenv("KUBECONFIG"); kubeconfigEnv != "" {
		kubeconfigPath = kubeconfigEnv
	} else {
		var homedir string
		homedir, err = os.UserHomeDir()
		if err != nil {
			return nil, err
		}
		kubeconfigPath = filepath.Join(homedir, ".kube", "config")
	}

	return buildClientsetWithPath(kubeconfigPath)

}

func buildClientsetWithPath(path string) (*kubernetes.Clientset, error) {
	config, err := clientcmd.BuildConfigFromFlags("", path)
	if err != nil {
		return nil, err
	}
	return kubernetes.NewForConfig(config)
}
