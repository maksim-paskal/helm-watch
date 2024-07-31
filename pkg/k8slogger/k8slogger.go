package k8slogger

import (
	"bufio"
	"context"
	"fmt"
	"slices"
	"strings"
	"sync"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

const (
	annotationPrefix = "helm-watch"
	// required annotation, list of containers to print to watch logs.
	annotationContainers = annotationPrefix + "/containers"

	// optional annotation, release name to filter logs.
	annotationReleaseName = annotationPrefix + "/release-name"
)

func NewPodLogger() *PodLogger {
	return &PodLogger{
		PodLabelSelector: "batch.kubernetes.io/job-name",
		TailLines:        10, //nolint:mnd
		SinceSeconds:     1,
	}
}

type PodLogger struct {
	Clientset        *kubernetes.Clientset
	ReleaseName      string
	Namespace        string
	PodLabelSelector string
	TailLines        int64
	SinceSeconds     int64
	watchedPods      sync.Map
}

func (l *PodLogger) printLogs(ctx context.Context, podName, container string) error {
	mapKey := podName + container

	if _, ok := l.watchedPods.Load(mapKey); ok {
		logrus.Debugf("pod %s already watched", mapKey)

		return nil
	}

	request := l.Clientset.CoreV1().Pods(l.Namespace).GetLogs(podName, &corev1.PodLogOptions{
		Container:    container,
		Follow:       true,
		SinceSeconds: &l.SinceSeconds,
		TailLines:    &l.TailLines,
	})

	podLogs, err := request.Stream(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get logs")
	}
	defer podLogs.Close()

	l.watchedPods.Store(mapKey, true)
	defer l.watchedPods.Delete(mapKey)

	logrus.Infof("Watching pod %s/%s", podName, container)

	for ctx.Err() == nil {
		scanner := bufio.NewScanner(podLogs)

		for scanner.Scan() {
			fmt.Printf("(%s/%s) %s\n", podName, container, scanner.Text()) //nolint:forbidigo
		}
	}

	return nil
}

// return valid containers.
func (l *PodLogger) checkPodAnnotations(annotations map[string]string) ([]string, bool) {
	containersText, ok := annotations[annotationContainers]
	if !ok {
		logrus.Debug("pod has no annotation")

		return nil, false
	}

	if releaseName, ok := annotations[annotationReleaseName]; ok && releaseName != l.ReleaseName {
		logrus.Debugf("release name %s is not %s", releaseName, l.ReleaseName)

		return nil, false
	}

	return strings.Split(containersText, ","), true
}

func (l *PodLogger) run(ctx context.Context) error {
	watcher, err := l.Clientset.CoreV1().Pods(l.Namespace).Watch(ctx, metav1.ListOptions{
		LabelSelector: l.PodLabelSelector,
		Watch:         true,
	})
	if err != nil {
		return errors.Wrap(err, "failed to list pods")
	}

	for event := range watcher.ResultChan() {
		pod, ok := event.Object.(*corev1.Pod)
		if !ok {
			logrus.Debug("event is not a pod")

			continue
		}

		validContainers, ok := l.checkPodAnnotations(pod.Annotations)
		if !ok {
			logrus.Debug("pod has no annotation")

			continue
		}

		for _, container := range pod.Spec.Containers {
			if !slices.Contains(validContainers, container.Name) {
				logrus.Debugf("container %s is not in list", container.Name)

				continue
			}

			if pod.Status.Phase != corev1.PodRunning {
				logrus.Debug("pod is not running")

				continue
			}

			go func(container corev1.Container) {
				if err := l.printLogs(ctx, pod.Name, container.Name); err != nil {
					logrus.Error(err)
				}
			}(container)
		}
	}

	return nil
}

func (l *PodLogger) Start(ctx context.Context) {
	logrus.Infof("Using namespace: %s", l.Namespace)
	logrus.Infof("Using release-name: %s", l.ReleaseName)
	logrus.Infof("Using pods filter: %s", l.PodLabelSelector)

	if err := l.run(ctx); err != nil {
		logrus.Error(err)
	}
}
