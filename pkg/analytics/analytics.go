package analytics

import (
	"bytes"
	"encoding/json"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path"
	"runtime"
	"sync"
	"time"

	"github.com/cloudnativedevelopment/cnd/pkg/config"
	"github.com/cloudnativedevelopment/cnd/pkg/log"
	"github.com/denisbrodbeck/machineid"
	uuid "github.com/satori/go.uuid"
)

//EventName event name
type EventName string

type event struct {
	//ActionID to correlate different events
	ActionID string `json:"action"`

	//Event name of the event
	Event EventName `json:"event"`

	//User local id of the client
	User string `json:"uid"`

	//Time time of the event
	Time int64 `json:"time"`

	//Version of the cli
	Version string `json:"version"`

	//OS of the user
	OS string `json:"os"`
}

var (
	userID string

	client = http.Client{
		Timeout: 65 * time.Second,
	}

	wg = sync.WaitGroup{}
)

const (
	// EventUp event for up
	EventUp = "up"

	// EventUpEnd event for when up finishes
	EventUpEnd = "upend"

	// EventExec event for exec
	EventExec = "exec"

	// EventExecEnd event for when exec finishes
	EventExecEnd = "execend"

	// EventRun event for run
	EventRun = "run"

	// EventRunEnd event for when run finishes
	EventRunEnd = "runend"
)

func init() {
	var err error
	userID, err = machineid.ProtectedID("cnd")
	if err != nil {
		log.Debugf("failed to generate a machine id")
		userID = "na"
	}
}

// NewActionID returns an action
func NewActionID() string {
	return uuid.NewV4().String()
}

// Send send analytics event
func Send(e EventName, actionID string) {
	go func() {
		wg.Add(1)
		defer wg.Done()

		if !isEnabled() {
			return
		}

		ev := event{
			ActionID: actionID,
			Event:    e,
			Time:     time.Now().UTC().Unix(),
			Version:  config.VersionString,
			User:     userID,
			OS:       runtime.GOOS,
		}

		data, err := json.Marshal(ev)
		if err != nil {
			log.Debugf("[%s] failed to marshall analytic event: %s", actionID, err)
			return
		}

		log.Debugf("[%s] sending analytics: %s", actionID, string(data))
		req, _ := http.NewRequest("POST", config.GetAnalyticsEndpoint(), bytes.NewBuffer(data))
		req.Header.Set("Content-Type", "application/json")
		resp, err := client.Do(req)

		if err != nil {
			log.Debugf("[%s] failed to send the analytics: %s", actionID, err)
			return
		}

		io.Copy(ioutil.Discard, resp.Body)
		defer resp.Body.Close()

		if resp.StatusCode > 300 {
			log.Debugf("[%s] analytics fail to process request: %d", actionID, resp.StatusCode)
			return
		}
	}()
}

// Disable disables analytics
func Disable() error {
	var _, err = os.Stat(getFlagPath())
	if os.IsNotExist(err) {
		var file, err = os.Create(getFlagPath())
		if err != nil {
			return err
		}

		defer file.Close()
	}

	return nil
}

// Enable enables analytics
func Enable() error {
	var _, err = os.Stat(getFlagPath())
	if os.IsNotExist(err) {
		return nil
	}

	return os.Remove(getFlagPath())
}

func isEnabled() bool {
	if _, err := os.Stat(getFlagPath()); !os.IsNotExist(err) {
		return false
	}

	return true
}

// Wait for the analytics to be finished
func Wait() {
	if !isEnabled() {
		return
	}
	log.Debug("waiting for analytics...")

	waitCh1 := make(chan struct{})
	go func() {
		wg.Wait()
		close(waitCh1)
	}()

	waitCh2 := make(chan struct{})
	go func() {
		time.Sleep(1 * time.Second)
		close(waitCh2)
	}()

	select {
	case <-waitCh1:
		log.Debug("all analytics were sent")
	case <-waitCh2:
		log.Debug("some analytics were not sent before shutdown")
		return
	}
}

func getFlagPath() string {
	return path.Join(config.GetCNDHome(), ".noanalytics")
}
