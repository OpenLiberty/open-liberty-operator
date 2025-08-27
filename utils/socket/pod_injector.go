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
	"github.com/go-logr/logr"
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
	MinWorkers         = 1
	MaxWorkers         = 100
	BuildMessageLength = 5
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
	conn   net.Conn
	logger logr.Logger
}

func (c *Client) Connect() error {
	conn, err := net.Dial("unix", podInjectorSocketPath)
	if err != nil {
		return err
	}
	c.conn = conn
	return nil
}

func buildMessage(podName, podNamespace, scriptName, podInjectorAction, payload string) []byte {
	return []byte(fmt.Sprintf("%s:%s:%s:%s:%s\n", podName, podNamespace, scriptName, podInjectorAction, payload))
}

func (c *Client) PollStatus(scriptName, podName, podNamespace, encodedAttrs string) string {
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
			c.logger.Info(fmt.Sprintf("PollStatus: Received message: %s", msg))
			output <- msg
			break
		}
	}()
	c.conn.Write(buildMessage(podName, podNamespace, scriptName, PodInjectorActionStatus, encodedAttrs))
	wg.Wait()
	return <-output
}

func (c *Client) PollLinperfFileName(scriptName, podName, podNamespace, attrs string) string {
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
			c.logger.Info(fmt.Sprintf("PollLinperfFileName: Received message: %s", msg))
			output <- msg
			break
		}
	}()
	c.conn.Write(buildMessage(podName, podNamespace, scriptName, PodInjectorActionLinperfFileName, attrs))
	wg.Wait()
	return <-output
}

func (c *Client) write(msg []byte) {
	if c.conn == nil {
		return
	}
	c.conn.Write(msg)
}

func (c *Client) StartScript(scriptName, podName, podNamespace, attrs string) bool {
	c.write(buildMessage(podName, podNamespace, scriptName, PodInjectorActionStart, attrs))
	return true
}

func (c *Client) CompleteScript(scriptName, podName, podNamespace, attrs string) {
	c.write(buildMessage(podName, podNamespace, scriptName, PodInjectorActionComplete, attrs))
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
	output := make(chan string, 1)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		scanner := bufio.NewScanner(c.conn)
		for scanner.Scan() {
			msg := scanner.Text()
			c.logger.Info(fmt.Sprintf("SetMaxWorkers: Received message: %s", msg))
			output <- msg
			break
		}
	}()
	c.conn.Write(buildMessage(podName, podNamespace, scriptName, PodInjectorActionSetMaxWorkers, maxWorkers))
	wg.Wait()
	return <-output == string(PodInjectorStatusUpdateMaxWorkersSuccess)
}

func GetPodInjectorClient(logger logr.Logger) *Client {
	return &Client{
		logger: logger,
	}
}

var _ utils.PodInjectorClient = (*Client)(nil)

func ServePodInjector(mgr manager.Manager, logger logr.Logger) (net.Listener, error) {
	os.Remove(podInjectorSocketPath)
	logger.Info(fmt.Sprintf("Creating socket at path: %s", podInjectorSocketPath))
	listener, err := net.Listen("unix", podInjectorSocketPath)
	if err != nil {
		return nil, err
	}

	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				logger.Error(err, "Failed to accept connection")
				continue
			}
			go handleConnection(mgr, conn, logger)
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

func processAction(conn net.Conn, mgr manager.Manager, logger logr.Logger, podName, podNamespace, tool, action, encodedAttrs string) {
	if len(podName) == 0 || len(podNamespace) == 0 {
		return
	}
	podKeyPair := getPodKey(podName, podNamespace)
	decodedLinperfAttrs := utils.DecodeLinperfAttr(encodedAttrs)
	podKey := fmt.Sprintf("%s:%s", podKeyPair, decodedLinperfAttrs["uid"])
	debugLogSignature := fmt.Sprintf("pod (%s), namespace (%s), active workers: (%d)", podName, podNamespace, len(workers))
	logger.V(2).Info(fmt.Sprintf("processAction: start: [%s]", debugLogSignature))
	defer func() {
		debugLogSignature := fmt.Sprintf("pod (%s), namespace (%s), active workers: (%d)", podName, podNamespace, len(workers))
		logger.V(2).Info(fmt.Sprintf("processAction: end: [%s]", debugLogSignature))
	}()

	switch action {
	case PodInjectorActionSetMaxWorkers:
		desiredWorkers, err := strconv.Atoi(encodedAttrs)
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
		reader, writer, cancelContext, err := CopyAndRunLinperf(mgr.GetConfig(), podName, podNamespace, encodedAttrs, func(stdout string, stderr string, err error) {
			removeWorker(podKey)
			if err == nil {
				logger.Info("The linperf script has completed successfully!")
				logger.Info("> linperf.sh (stdout):")
				logger.Info(stdout)
				logger.Info("> linperf.sh (stderr):")
				logger.Info(stderr)
				completedPods.Store(podKey, true)
				fileName := getLinperfDataFileName(stdout)
				linperfFileNames.Store(podKey, fileName)
			} else {
				errMessage := fmt.Sprintf("The performance data collector failed with error: %s", err)
				logger.Error(err, "The performance data collector failed")
				logger.Info("> linperf.sh (stdout):")
				logger.Info(stdout)
				logger.Info("> linperf.sh (stderr):")
				logger.Info(stderr)
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

func handleConnection(mgr manager.Manager, conn net.Conn, logger logr.Logger) {
	defer conn.Close()

	buffer := make([]byte, 1024)
	for {
		n, err := conn.Read(buffer)
		if err != nil {
			// logger.Error(fmt.Errorf("Failed to populate data into the buffer, skipping..."), "Invalid message")
			return
		}
		messagesString := string(buffer[:n])
		messages := strings.Split(messagesString, "\n")
		for _, message := range messages {
			message = strings.Trim(message, " ")
			if len(message) == 0 {
				// logger.Error(fmt.Errorf("Expected an non-empty message but received nothing, skipping..."), "Invalid message")
				continue
			}
			messageArr := strings.Split(message, ":")
			if len(messageArr) != BuildMessageLength {
				logger.Error(fmt.Errorf("Expected len(messageArr) == %d but got length %d", BuildMessageLength, len(messageArr)), "Invalid message")
				return
			}
			podName, podNamespace, tool, action, encodedAttrs := messageArr[0], messageArr[1], messageArr[2], messageArr[3], messageArr[4]

			debugLogSignature := fmt.Sprintf("tool (%s), action (%s), pod (%s), namespace (%s), payload (%s)", tool, action, podName, podNamespace, encodedAttrs)
			logger.V(2).Info(fmt.Sprintf("Requesting lock: [%s]", debugLogSignature))
			mutex.Lock()
			logger.V(2).Info(fmt.Sprintf("Holding critical section: [%s]", debugLogSignature))
			processAction(conn, mgr, logger, podName, podNamespace, tool, action, encodedAttrs)
			logger.V(2).Info(fmt.Sprintf("Releasing lock: [%s]", debugLogSignature))
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
