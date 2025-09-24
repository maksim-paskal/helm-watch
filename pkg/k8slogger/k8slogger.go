package k8slogger

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"slices"
	"strings"
	"sync"
	"time"

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
		SinceSeconds:     1,
		PrintExtended:    os.Getenv("HELM_WATCH_EXTENDED") == "true",
	}
}

type PodLogger struct {
	Clientset        *kubernetes.Clientset
	ReleaseName      string
	Namespace        string
	PodLabelSelector string
	SinceSeconds     int64

	watchedPods       sync.Map
	watchedContainers sync.Map
	watchedEvents     sync.Map

	PrintExtended  bool
	FatalOnPodFail bool
}

func (l *PodLogger) printEventsLogs(ctx context.Context, podName, container string) error {
	mapKey := podName + container

	if _, ok := l.watchedEvents.Load(mapKey); ok {
		logrus.Debugf("events %s already watched", mapKey)

		return nil
	}

	l.watchedEvents.Store(mapKey, true)

	opts := metav1.ListOptions{
		FieldSelector: fmt.Sprintf("involvedObject.name=%s,involvedObject.namespace=%s", podName, l.Namespace),
		Watch:         true,
	}

	logrus.Infof("Watching events %s", opts.FieldSelector)

	watcher, err := l.Clientset.CoreV1().Events(l.Namespace).Watch(ctx, opts)
	if err != nil {
		return errors.Wrap(err, "failed to list events")
	}

	for event := range watcher.ResultChan() {
		event, ok := event.Object.(*corev1.Event)
		if !ok {
			logrus.Debug("event is not a corev1.Event")

			continue
		}

		l.print(podName, container, event.Message)
	}

	return nil
}

func (l *PodLogger) print(podName, container, message string) {
	now := time.Now().Format(time.TimeOnly)

	if l.PrintExtended {
		fmt.Printf("[%s](%s/%s) %s\n", now, podName, container, message) //nolint:forbidigo
	} else {
		fmt.Printf("[%s] %s\n", now, message) //nolint:forbidigo
	}
}

func (l *PodLogger) printContainerLogs(ctx context.Context, podName, container string) error {
	mapKey := podName + container

	if _, ok := l.watchedContainers.Load(mapKey); ok {
		logrus.Debugf("pod %s already watched", mapKey)

		return nil
	}

	request := l.Clientset.CoreV1().Pods(l.Namespace).GetLogs(podName, &corev1.PodLogOptions{
		Container:    container,
		Follow:       true,
		SinceSeconds: &l.SinceSeconds,
	})

	podLogs, err := request.Stream(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get logs")
	}
	defer podLogs.Close()

	l.watchedContainers.Store(mapKey, true)
	defer l.watchedContainers.Delete(mapKey)

	logrus.Infof("Watching pod %s/%s", podName, container)

	for ctx.Err() == nil {
		scanner := bufio.NewScanner(podLogs)

		for scanner.Scan() {
			l.print(podName, container, scanner.Text())
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

// check if pod is failed.
func (l *PodLogger) checkPodStatus(pod *corev1.Pod) {
	// if pod is watched and failed, exit.
	if _, ok := l.watchedPods.Load(pod.Name); ok && pod.Status.Phase == corev1.PodFailed {
		message := fmt.Sprintf("pod %s/%s has failed", pod.Namespace, pod.Name)

		if l.FatalOnPodFail {
			logrus.Fatal(message)
		} else {
			logrus.Warn(message)
		}
	}
}

func (l *PodLogger) run(ctx context.Context) error {
	opts := metav1.ListOptions{
		LabelSelector: l.PodLabelSelector,
		Watch:         true,
	}

	logrus.Infof("Watching pods %s", opts.LabelSelector)

	watcher, err := l.Clientset.CoreV1().Pods(l.Namespace).Watch(ctx, opts)
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

		l.checkPodStatus(pod)

		// watch only running pods
		if pod.Status.Phase != corev1.PodRunning {
			logrus.Debug("pod is not running")

			continue
		}

		// remember that we are watching this pod
		l.watchedPods.Store(pod.Name, true)

		for _, container := range pod.Spec.Containers {
			if !slices.Contains(validContainers, container.Name) {
				logrus.Debugf("container %s is not in list", container.Name)

				continue
			}

			go func(container corev1.Container) {
				if err := l.printEventsLogs(ctx, pod.Name, container.Name); err != nil {
					logrus.Error(err)
				}
			}(container)

			go func(container corev1.Container) {
				if err := l.printContainerLogs(ctx, pod.Name, container.Name); err != nil {
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

	if err := l.run(ctx); err != nil {
		logrus.Error(err)
	}
}
