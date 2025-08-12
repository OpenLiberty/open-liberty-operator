package socket

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path"
	"strings"

	_ "unsafe"

	"github.com/OpenLiberty/open-liberty-operator/utils"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/remotecommand"
	"k8s.io/kubectl/pkg/scheme"
)

//go:linkname cpMakeTar k8s.io/kubectl/pkg/cmd/cp.makeTar
func cpMakeTar(srcPath, destPath string, writer io.Writer) error

func CopyAndRunLinperf(restConfig *rest.Config, podName string, podNamespace string, encodedAttr string, doneCallback func(string, string, error)) (*io.PipeReader, *io.PipeWriter, context.CancelFunc, error) {
	containerName := "app"
	sourceFolder := "internal/controller/assets/helper"
	destFolder := "/output/helper"
	linperfCmd := utils.GetLinperfCmd(encodedAttr, podName, podNamespace)
	return CopyFolderToPodAndRunScript(restConfig, sourceFolder, destFolder, podName, podNamespace, containerName, linperfCmd, doneCallback)
}

// Gets the linperf data file name from the stdout output of the linperf.sh script
func getLinperfDataFileName(linperfOutput string) string {
	parentDir := "/serviceability"
	fileType := ".tar.gz"
	for _, line := range strings.Split(linperfOutput, "\n") {
		if strings.Contains(line, parentDir) && strings.Contains(line, fileType) {
			startIndex := strings.Index(line, parentDir)
			endIndex := strings.Index(line, fileType)
			if startIndex != -1 && endIndex != -1 && startIndex < len(line) && endIndex+len(fileType) <= len(line) {
				return line[startIndex : endIndex+len(fileType)]
			}
		}
	}
	return ""
}

func podExec(clientset *kubernetes.Clientset, podName, podNamespace, containerName string, usingStdin bool, command []string) *rest.Request {
	return clientset.CoreV1().RESTClient().Post().
		Resource("pods").
		Name(podName).
		Namespace(podNamespace).
		SubResource("exec").
		VersionedParams(&corev1.PodExecOptions{
			Command:   command,
			Container: containerName,
			Stdin:     usingStdin,
			Stdout:    true,
			Stderr:    true,
			TTY:       false,
		}, scheme.ParameterCodec)
}

func CopyFolderToPodAndRunScript(config *rest.Config, srcFolder string, destFolder string, podName, podNamespace, containerName, scriptCmd string, doneCallback func(string, string, error)) (*io.PipeReader, *io.PipeWriter, context.CancelFunc, error) {
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("Failed to create Clientset: %v", err.Error())
	}

	reader, writer := io.Pipe()
	go func() {
		defer writer.Close()
		cpMakeTar(srcFolder, destFolder, writer)
	}()

	command := []string{"tar", "-xf", "-"}
	destDir := path.Dir(destFolder)
	if len(destDir) > 0 {
		command = append(command, "-C", destDir)
	}

	streamContext, cancelStreamContext := context.WithCancel(context.Background())
	go func() {
		usingStdin := true
		exec, err := remotecommand.NewSPDYExecutor(config, "POST", podExec(clientset, podName, podNamespace, containerName, usingStdin, command).URL())
		if err != nil {
			doneCallback("", "", err)
			return
		}
		err = exec.StreamWithContext(streamContext, remotecommand.StreamOptions{
			Stdin:  reader,
			Stdout: os.Stdout,
			Stderr: os.Stderr,
			Tty:    false,
		})

		usingStdin = false
		exec, err = remotecommand.NewSPDYExecutor(config, "POST", podExec(clientset, podName, podNamespace, containerName, usingStdin, []string{"/bin/sh", "-c", scriptCmd}).URL())
		var stdout, stderr bytes.Buffer
		err = exec.StreamWithContext(streamContext, remotecommand.StreamOptions{
			Stdout: &stdout,
			Stderr: &stderr,
			Tty:    false,
		})
		doneCallback(stdout.String(), stderr.String(), err)
	}()
	return reader, writer, cancelStreamContext, nil
}
