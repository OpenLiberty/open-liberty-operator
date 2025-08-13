package socket

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"

	"github.com/OpenLiberty/open-liberty-operator/utils"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

type PodInjectorStatusResponse string

const (
	podInjectorSocketPath                                                      = "/tmp/operator.sock"
	PodInjectorActionSetMaxWorkers                                             = "setMaxWorkers"
	PodInjectorActionStart                                                     = "start"
	PodInjectorActionComplete                                                  = "complete"
	PodInjectorActionStop                                                      = "stop"
	PodInjectorActionStatus                                                    = "status"
	PodInjectorActionLinperfFileName                                           = "linperfFileName"
	PodInjectorStatusUpdateMaxWorkersSuccess         PodInjectorStatusResponse = "updateMaxWorkersSuccess..."
	PodInjectorStatusUpdateMaxWorkersBusy            PodInjectorStatusResponse = "updateMaxWorkersBusy..."
	PodInjectorStatusUpdateMaxWorkersInvalidArgument PodInjectorStatusResponse = "updateMaxWorkersInvalidArgument..."
	PodInjectorStatusWriting                         PodInjectorStatusResponse = "writing..."
	PodInjectorStatusIdle                            PodInjectorStatusResponse = "idle..."
	PodInjectorStatusDone                            PodInjectorStatusResponse = "done..."
	PodInjectorStatusClosed                          PodInjectorStatusResponse = "closed..."
	PodInjectorStatusNotFound                        PodInjectorStatusResponse = "notfound..."
	PodInjectorStatusTooManyWorkers                  PodInjectorStatusResponse = "toomanyworkers..."
)

const (
	MinWorkers = 1
	MaxWorkers = 100
)

var (
	mutex             = &sync.Mutex{}
	workers           = []Worker{}
	completedPods     = &sync.Map{}
	erroringPods      = &sync.Map{}
	linperfFileNames  = &sync.Map{}
	currentMaxWorkers = 10
)

type Worker struct {
	reader        *io.PipeReader
	writer        *io.PipeWriter
	cancelContext context.CancelFunc
	podKey        string
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
	c.conn.Write([]byte(fmt.Sprintf("%s:%s:%s:%s:%s\n", podName, podNamespace, scriptName, PodInjectorActionStatus, "")))
	wg.Wait()
	return <-output
}

func (c *Client) PollLinperfFileName(scriptName, podName, podNamespace string) string {
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
	c.conn.Write([]byte(fmt.Sprintf("%s:%s:%s:%s:%s\n", podName, podNamespace, scriptName, PodInjectorActionLinperfFileName, "")))
	wg.Wait()
	return <-output
}

func (c *Client) StartScript(scriptName, podName, podNamespace, attrs string) bool {
	if c.conn == nil {
		return false
	}
	c.conn.Write([]byte(fmt.Sprintf("%s:%s:%s:%s:%s\n", podName, podNamespace, scriptName, PodInjectorActionStart, attrs)))
	return true
}

func (c *Client) CompleteScript(scriptName, podName, podNamespace string) {
	if c.conn == nil {
		return
	}
	c.conn.Write([]byte(fmt.Sprintf("%s:%s:%s:%s:%s\n", podName, podNamespace, scriptName, PodInjectorActionComplete, "")))
}

func (c *Client) CloseConnection() {
	if c.conn == nil {
		return
	}
	c.conn.Close()
}

func (c *Client) SetMaxWorkers(scriptName, podName, podNamespace, maxWorkers string) bool {
	if c.conn == nil {
		return false
	}
	c.conn.Write([]byte(fmt.Sprintf("%s:%s:%s:%s:%s\n", podName, podNamespace, scriptName, PodInjectorActionSetMaxWorkers, maxWorkers)))
	return true
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

func getPodKey(podName, podNamespace string) string {
	return fmt.Sprintf("%s:%s", podNamespace, podName)
}

func processAction(conn net.Conn, mgr manager.Manager, podName, podNamespace, tool, action, encodedAttr string) {
	if len(podName) == 0 || len(podNamespace) == 0 {
		return
	}
	podKey := getPodKey(podName, podNamespace)
	switch action {
	case PodInjectorActionSetMaxWorkers:
		desiredWorkers, err := strconv.Atoi(encodedAttr)
		// Exit early if desired workers is out of bounds
		if err != nil || desiredWorkers < MinWorkers || desiredWorkers > MaxWorkers {
			writeResponse(conn, PodInjectorStatusUpdateMaxWorkersInvalidArgument)
			return
		}
		// Update currentMaxWorkers as needed
		if desiredWorkers != currentMaxWorkers {
			activeWorkers := len(workers)
			currentMaxWorkers = max(activeWorkers, desiredWorkers)
		}
		if desiredWorkers == currentMaxWorkers {
			writeResponse(conn, PodInjectorStatusUpdateMaxWorkersSuccess)
		} else {
			writeResponse(conn, PodInjectorStatusUpdateMaxWorkersBusy)
		}
	case PodInjectorActionStart:
		if hasWorker(podKey) {
			writeResponse(conn, PodInjectorStatusWriting)
			return
		} else if len(workers) >= currentMaxWorkers {
			writeResponse(conn, PodInjectorStatusTooManyWorkers)
			return
		}
		completedPods.Store(podKey, false)
		linperfFileNames.Delete(podKey)
		reader, writer, cancelContext, err := CopyAndRunLinperf(mgr.GetConfig(), podName, podNamespace, encodedAttr, func(stdout string, stderr string, err error) {
			removeWorker(podKey)
			if err == nil {
				fmt.Println("The linperf script has completed successfully.")
				fmt.Println(stdout)
				fmt.Println(stderr)
				completedPods.Store(podKey, true)
				fileName := getLinperfDataFileName(stdout)
				linperfFileNames.Store(podKey, fileName)
			} else {
				errMessage := fmt.Sprintf("The performance data collector has failed with error: %s", err)
				fmt.Println(errMessage)
				fmt.Println(stdout)
				fmt.Println(stderr)
				erroringPods.Store(podKey, errMessage)
			}
		})
		if err == nil {
			workers = append(workers, Worker{
				reader:        reader,
				writer:        writer,
				cancelContext: cancelContext,
				podKey:        podKey,
			})
		}
		writeResponse(conn, PodInjectorStatusWriting)
	case PodInjectorActionComplete:
		removeWorker(podKey)
		completedPods.Delete(podKey)
		erroringPods.Delete(podKey)
	case PodInjectorActionStatus:
		if hasWorker(podKey) {
			writeResponse(conn, PodInjectorStatusWriting)
		} else if value, ok := erroringPods.Load(podKey); ok {
			writeResponse(conn, PodInjectorStatusResponse(fmt.Sprintf("error:%s", value.(string))))
		} else if value, ok := completedPods.Load(podKey); ok && value.(bool) {
			writeResponse(conn, PodInjectorStatusDone)
		} else if len(workers) >= currentMaxWorkers {
			writeResponse(conn, PodInjectorStatusTooManyWorkers)
		} else {
			writeResponse(conn, PodInjectorStatusIdle)
		}
	case PodInjectorActionLinperfFileName:
		if value, ok := linperfFileNames.Load(podKey); ok {
			writeResponse(conn, PodInjectorStatusResponse(fmt.Sprintf("name:%s", value.(string))))
		} else {
			writeResponse(conn, PodInjectorStatusNotFound)
		}
	case PodInjectorActionStop:
		removeWorker(podKey)
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

func hasWorker(podKey string) bool {
	for _, worker := range workers {
		if worker.podKey == podKey {
			return true
		}
	}
	return false
}

func removeWorker(podKey string) {
	// find index of the worker
	deleteIndex := -1
	for i, worker := range workers {
		if worker.podKey == podKey {
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
