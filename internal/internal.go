package internal

import (
	"context"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"github.com/maksim-paskal/helm-watch/pkg/k8slogger"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

type Application struct {
	Args        []string
	clientset   *kubernetes.Clientset
	namespace   string
	releaseName string
}

func NewApplication() *Application {
	return &Application{}
}

func (a *Application) Init() error {
	if len(a.Args) == 0 {
		return errors.New("no command provided")
	}

	kubeConfig, err := clientcmd.BuildConfigFromFlags("", os.Getenv("KUBECONFIG"))
	if err != nil {
		return errors.Wrap(err, "failed to build kubeconfig")
	}

	a.clientset, err = kubernetes.NewForConfig(kubeConfig)
	if err != nil {
		return errors.Wrap(err, "failed to build kubeconfig")
	}

	a.namespace = a.getNamespace()

	if len(a.namespace) == 0 {
		a.namespace = "default"
	}

	a.releaseName = a.getReleaseName()

	if len(a.releaseName) == 0 {
		return errors.New("no release name provided")
	}

	return nil
}

func (a *Application) getFlagValue(flag, defaultValue string) string {
	re2 := regexp.MustCompile(`(` + flag + `)[ =][\"\']?([\w-]+)[\"\']?`)
	cmd := strings.Join(a.Args, " ")

	if !re2.MatchString(cmd) {
		return defaultValue
	}

	return re2.FindStringSubmatch(cmd)[2]
}

func (a *Application) getReleaseName() string {
	if a.Args[0] == "helm" && len(a.Args) >= 3 && (a.Args[1] == "upgrade" || a.Args[1] == "install") {
		return a.Args[2]
	}

	return a.getFlagValue("--release-name", os.Getenv("RELEASE_NAME"))
}

func (a *Application) getNamespace() string {
	return a.getFlagValue("-n|--namespace", os.Getenv("NAMESPACE"))
}

func (a *Application) runInternalWaitForJobs(ctx context.Context) error { //nolint:cyclop,funlen
	filter := a.getFlagValue("--filter", "")
	if filter == "" {
		return errors.New("no filter provided")
	}

	logrus.Info("Using filter: ", filter)

	hasFailedJobs := false

	logrus.Info("Waiting for jobs...")

	for ctx.Err() == nil {
		jobs, err := a.clientset.BatchV1().Jobs(a.namespace).List(ctx, metav1.ListOptions{
			LabelSelector: filter,
		})
		if err != nil {
			return errors.Wrap(err, "failed to watch jobs")
		}

		total := len(jobs.Items)
		succeeded := 0
		failed := 0
		hasFailedJobs = false

		logrus.Debug("Total jobs: ", total)

		for _, job := range jobs.Items {
			logrus := logrus.WithFields(logrus.Fields{
				"job": job.Name,
			})

			if job.Status.Succeeded > 0 {
				logrus.Debug("Job succeeded")

				succeeded++
			}

			if job.Status.Failed > 0 {
				logrus.Debug("Job succeeded")

				failed++
			}
		}

		if failed > 0 {
			logrus.Debug("hasFailedJobs")

			hasFailedJobs = true
		}

		if total == succeeded+failed {
			logrus.Info("All jobs finished")

			break
		}

		select {
		case <-ctx.Done():
		case <-time.After(time.Second):
		}
	}

	if hasFailedJobs {
		return errors.New("some jobs failed")
	}

	return nil
}

func (a *Application) runInternal(ctx context.Context) error {
	if len(a.Args) < 2 { //nolint:mnd
		return errors.New("no internal command provided")
	}

	switch a.Args[1] {
	case "wait-for-jobs":
		return a.runInternalWaitForJobs(ctx)
	default:
		return errors.Errorf("unknown %s internal command", a.Args[1])
	}
}

func (a *Application) runShellCommand(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, a.Args[0], a.Args[1:]...) //nolint:gosec

	logrus.Info("Running command: ", cmd.String())
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return errors.Wrap(err, "failed to run command")
	}

	return nil
}

func (a *Application) Run(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	jobLogger := k8slogger.NewJobLogger()

	jobLogger.Clientset = a.clientset
	jobLogger.Namespace = a.namespace
	jobLogger.ReleaseName = a.releaseName

	go jobLogger.Start(ctx)

	switch a.Args[0] {
	case "internal":
		if err := a.runInternal(ctx); err != nil {
			return errors.Wrap(err, "failed to run internal command")
		}
	default:
		if err := a.runShellCommand(ctx); err != nil {
			return errors.Wrap(err, "failed to execute shell command")
		}
	}

	return nil
}
