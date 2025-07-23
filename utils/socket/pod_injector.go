package socket

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"strings"
	"sync"

	"github.com/OpenLiberty/open-liberty-operator/utils"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

type PodInjectorStatusResponse string

const (
	podInjectorSocketPath                                     = "/tmp/operator.sock"
	PodInjectorActionStart                                    = "start"
	PodInjectorActionStop                                     = "stop"
	PodInjectorActionStatus                                   = "status"
	PodInjectorStatusWriting        PodInjectorStatusResponse = "writing..."
	PodInjectorStatusIdle           PodInjectorStatusResponse = "idle..."
	PodInjectorStatusDone           PodInjectorStatusResponse = "done..."
	PodInjectorStatusClosed         PodInjectorStatusResponse = "closed..."
	PodInjectorStatusTooManyWorkers PodInjectorStatusResponse = "toomanyworkers..."
)

var (
	mutex         = &sync.Mutex{}
	workers       = []Worker{}
	completedPods = &sync.Map{}
	maxWorkers    = 1
)

type Worker struct {
	reader        *io.PipeReader
	writer        *io.PipeWriter
	cancelContext context.CancelFunc
	podName       string
}

type Client struct {
	conn net.Conn
}

func (c *Client) Connect() error {
	conn, err := net.Dial("unix", podInjectorSocketPath)
	if err != nil {
		return err
	}
	c.conn = conn
	return nil
}

func (c *Client) PollStatus(scriptName, podName, podNamespace string) string {
	if c.conn == nil {
		return string(PodInjectorStatusClosed)
	}
	output := make(chan string, 1)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		scanner := bufio.NewScanner(c.conn)
		for scanner.Scan() {
			msg := scanner.Text()
			fmt.Println("Message from pod injector: ", msg)
			output <- msg
			break
		}
	}()
	c.conn.Write([]byte(fmt.Sprintf("%s:%s:%s:%s:%s\n", podName, podNamespace, scriptName, "status", "")))
	wg.Wait()
	return <-output
}

func (c *Client) StartScript(scriptName, podName, podNamespace, attrs string) bool {
	if c.conn == nil {
		return false
	}
	c.conn.Write([]byte(fmt.Sprintf("%s:%s:%s:%s:%s\n", podName, podNamespace, scriptName, "start", attrs)))
	return true
}

func (c *Client) CloseConnection() {
	if c.conn == nil {
		return
	}
	c.conn.Close()
}

func GetPodInjectorClient() *Client {
	return &Client{}
}

var _ utils.PodInjectorClient = (*Client)(nil)

func ServePodInjector(mgr manager.Manager) (net.Listener, error) {
	os.Remove(podInjectorSocketPath)

	listener, err := net.Listen("unix", podInjectorSocketPath)
	if err != nil {
		return nil, err
	}

	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				fmt.Println("Connection error:", err)
				continue
			}
			go handleConnection(mgr, conn)
		}
	}()
	return listener, nil
}

func writeResponse(conn net.Conn, response PodInjectorStatusResponse) {
	conn.Write([]byte(fmt.Sprintf("%s\n", response)))
}

func processAction(conn net.Conn, mgr manager.Manager, podName, podNamespace, tool, action, encodedAttr string) {
	switch action {
	case PodInjectorActionStart:
		if hasWorker(podName) {
			writeResponse(conn, PodInjectorStatusWriting)
			return
		}
		if len(workers) >= maxWorkers {
			writeResponse(conn, PodInjectorStatusTooManyWorkers)
			return
		}
		completedPods.Store(podName, false)
		reader, writer, cancelContext, err := CopyAndRunLinperf(mgr.GetConfig(), podName, podNamespace, encodedAttr, func(err error) {
			removeWorker(podName)
			if err == nil {
				fmt.Println("The linperf script has completed successfully.")
				completedPods.Store(podName, true)
			} else {
				fmt.Println("The linperf script has failed with error: ", err)
			}
		})
		if err == nil {
			workers = append(workers, Worker{
				reader:        reader,
				writer:        writer,
				cancelContext: cancelContext,
				podName:       podName,
			})
		}
		writeResponse(conn, PodInjectorStatusWriting)
	case PodInjectorActionStatus:
		if hasWorker(podName) {
			writeResponse(conn, PodInjectorStatusWriting)
		} else if value, ok := completedPods.Load(podName); ok && value.(bool) {
			writeResponse(conn, PodInjectorStatusDone)
		} else {
			writeResponse(conn, PodInjectorStatusIdle)
		}
	case PodInjectorActionStop:
		removeWorker(podName)
	}
}

func handleConnection(mgr manager.Manager, conn net.Conn) {
	defer conn.Close()

	buffer := make([]byte, 1024)
	for {
		n, err := conn.Read(buffer)
		if err != nil {
			return
		}
		messagesString := string(buffer[:n])
		messages := strings.Split(messagesString, "\n")
		for _, message := range messages {
			message = strings.Trim(message, " ")
			if len(message) == 0 {
				continue
			}
			messageArr := strings.Split(message, ":")
			if len(messageArr) != 5 {
				return
			}
			podName, podNamespace, tool, action, encodedAttr := messageArr[0], messageArr[1], messageArr[2], messageArr[3], messageArr[4]
			mutex.Lock()
			processAction(conn, mgr, podName, podNamespace, tool, action, encodedAttr)
			mutex.Unlock()
		}
	}
}

func hasWorker(podName string) bool {
	for _, worker := range workers {
		if worker.podName == podName {
			return true
		}
	}
	return false
}

func removeWorker(podName string) {
	// find index of the worker
	deleteIndex := -1
	for i, worker := range workers {
		if worker.podName == podName {
			deleteIndex = i
			break
		}
	}
	// exit if worker is not found
	if deleteIndex == -1 {
		return
	}
	if workers[deleteIndex].cancelContext != nil {
		workers[deleteIndex].cancelContext()
	}
	if workers[deleteIndex].writer != nil {
		workers[deleteIndex].writer.Close()
	}
	if workers[deleteIndex].reader != nil {
		workers[deleteIndex].reader.Close()
	}
	workers = append(workers[:deleteIndex], workers[deleteIndex+1:]...)
}
