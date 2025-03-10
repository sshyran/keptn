package controller

import (
	"context"
	"errors"
	"fmt"
	"github.com/keptn/keptn/shipyard-controller/internal/common"
	"github.com/keptn/keptn/shipyard-controller/internal/controller/fake"
	"github.com/keptn/keptn/shipyard-controller/internal/db"
	shipyardRetrievermock "github.com/keptn/keptn/shipyard-controller/internal/shipyardretriever/fake"
	"os"
	"testing"
	"time"

	"github.com/benbjohnson/clock"
	apimodels "github.com/keptn/go-utils/pkg/api/models"
	keptnv2 "github.com/keptn/go-utils/pkg/lib/v0_2_0"
	"github.com/keptn/keptn/shipyard-controller/models"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tryvium-travels/memongo"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const testShipyardFileWithInvalidVersion = `apiVersion: 0
kind: Shipyard
metadata:
  name: test-shipyard`

const testShipyardFile = `apiVersion: spec.keptn.sh/0.2.0
kind: Shipyard
metadata:
  name: test-shipyard
spec:
  stages:
  - name: dev
    sequences:
    - name: artifact-delivery
      tasks:
      - name: deployment
        properties:  
          strategy: direct
      - name: test
        properties:
          kind: functional
      - name: evaluation 
      - name: release 
    - name: rollback
      tasks:
      - name: rollback
      triggeredOn:
        - event: dev.artifact-delivery.finished
          selector:
            match:
              result: fail
  - name: hardening
    sequences:
    - name: artifact-delivery
      triggeredOn:
        - event: dev.artifact-delivery.finished
      tasks:
      - name: deployment
        properties: 
          strategy: blue_green_service
      - name: test
        properties:  
          kind: performance
      - name: evaluation
      - name: release

  - name: production
    sequences:
    - name: artifact-delivery 
      triggeredOn:
        - event: hardening.artifact-delivery.finished
      tasks:
      - name: deployment
        properties:
          strategy: blue_green
      - name: release
      
    - name: remediation
      tasks:
      - name: remediation
      - name: evaluation`

const testShipyardFileWithDuplicateTasks = `apiVersion: spec.keptn.sh/0.2.2
kind: Shipyard
metadata:
  name: test-shipyard
spec:
  stages:
  - name: dev
    sequences:
    - name: artifact-delivery
      tasks:
      - name: deployment
      - name: deployment
      - name: evaluation`

const mongoDBVersion = "5.0.10"

func TestMain(m *testing.M) {
	defer setupLocalMongoDB()()
	m.Run()
}

func setupLocalMongoDB() func() {
	mongoServer, err := memongo.Start(mongoDBVersion)
	randomDbName := memongo.RandomDatabase()

	os.Setenv("MONGODB_DATABASE", randomDbName)
	os.Setenv("MONGODB_EXTERNAL_CONNECTION_STRING", fmt.Sprintf("%s/%s", mongoServer.URI(), randomDbName))

	var mongoDBClient *mongo.Client
	mongoDBClient, err = mongo.NewClient(options.Client().ApplyURI(mongoServer.URI()))
	if err != nil {
		log.Fatalf("Mongo Client setup failed: %s", err)
	}
	err = mongoDBClient.Connect(context.TODO())
	if err != nil {
		log.Fatalf("Mongo Server setup failed: %s", err)
	}

	return func() { mongoServer.Stop() }
}

//Scenario 1: Complete task sequence execution + triggering of next task sequence. Events are received in order
func Test_shipyardController_Scenario1(t *testing.T) {

	t.Logf("Executing Shipyard Controller Scenario 1 with shipyard file %s", testShipyardFile)
	sc, cancel := getTestShipyardController("")
	defer sc.StopDispatchers()
	defer cancel()
	projectName := "test-project"
	defer cleanupCollections(projectName, sc)

	mockDispatcher := sc.eventDispatcher.(*fake.IEventDispatcherMock)

	commitID := "my-commit-id"
	// STEP 1
	// send dev.artifact-delivery.triggered event
	sequenceTriggeredEvent := getArtifactDeliveryTriggeredEvent("dev", commitID)
	err := sc.HandleIncomingEvent(sequenceTriggeredEvent, true)
	if err != nil {
		t.Errorf("STEP 1 failed: HandleIncomingEvent(dev.artifact-delivery.triggered) returned %v", err)
		return
	}

	// check event dispatcher -> should contain deployment.triggered event with properties: [deployment]
	require.Eventually(t, func() bool {
		return len(mockDispatcher.AddCalls()) == 1
	}, 3*time.Second, 100*time.Millisecond)

	verifyEvent := mockDispatcher.AddCalls()[0].Event
	require.Equal(t, keptnv2.GetTriggeredEventType(keptnv2.DeploymentTaskName), verifyEvent.Event.Type())

	deploymentEvent := &keptnv2.DeploymentTriggeredEventData{}
	err = verifyEvent.Event.DataAs(deploymentEvent)
	require.Nil(t, err)
	require.Equal(t, 1, len(deploymentEvent.Deployment.DeploymentURIsPublic))
	require.Equal(t, "direct", deploymentEvent.Deployment.DeploymentStrategy)
	require.Equal(t, "carts", deploymentEvent.ConfigurationChange.Values["image"])

	// check triggeredEvent Collection -> should contain deployment.triggered event
	triggeredEvents, _ := sc.eventRepo.GetEvents("test-project", common.EventFilter{
		Type:    keptnv2.GetTriggeredEventType(keptnv2.DeploymentTaskName),
		Stage:   common.Stringp("dev"),
		Service: common.Stringp("carts"),
		Source:  common.Stringp("shipyard-controller"),
	}, common.TriggeredEvent)
	ShouldContainEvent(t, triggeredEvents, keptnv2.GetTriggeredEventType(keptnv2.DeploymentTaskName), "", nil)
	require.Equal(t, commitID, triggeredEvents[0].GitCommitID)
	triggeredID := triggeredEvents[0].ID

	// STEP 2
	// send deployment.started event
	sendAndVerifyStartedEvent(t, sc, keptnv2.DeploymentTaskName, triggeredID, "dev", "carts", "test-source")

	// STEP 3
	// send deployment.finished event
	triggeredID = sendAndVerifyFinishedEvent(
		t,
		sc,
		getDeploymentFinishedEvent("dev", "carts", triggeredID, "test-source", keptnv2.ResultPass),
		keptnv2.DeploymentTaskName,
		keptnv2.TestTaskName,
		"",
		"carts",
	)

	require.Equal(t, 2, len(mockDispatcher.AddCalls()))
	verifyEvent = mockDispatcher.AddCalls()[1].Event
	require.Equal(t, keptnv2.GetTriggeredEventType(keptnv2.TestTaskName), verifyEvent.Event.Type())
	require.Equal(t, commitID, verifyEvent.Event.Extensions()["gitcommitid"])

	taskEvent := &keptnv2.TestTriggeredEventData{}
	err = verifyEvent.Event.DataAs(taskEvent)
	require.Nil(t, err)
	require.Equal(t, 3, len(taskEvent.Deployment.DeploymentURIsPublic))
	require.Equal(t, 2, len(taskEvent.Deployment.DeploymentURIsLocal))

	// also check if the payload of the .triggered event that started the sequence is present
	deploymentEvent = &keptnv2.DeploymentTriggeredEventData{}
	err = verifyEvent.Event.DataAs(deploymentEvent)
	require.Equal(t, "carts", deploymentEvent.ConfigurationChange.Values["image"])

	// STEP 4
	// send test.started event
	sendAndVerifyStartedEvent(t, sc, keptnv2.TestTaskName, triggeredID, "dev", "carts", "test-source")

	// STEP 5
	// send test.finished event
	triggeredID = sendAndVerifyFinishedEvent(
		t,
		sc,
		getTestTaskFinishedEvent("dev", triggeredID),
		keptnv2.TestTaskName,
		keptnv2.EvaluationTaskName,
		"",
		"carts",
	)

	require.Equal(t, 3, len(mockDispatcher.AddCalls()))
	verifyEvent = mockDispatcher.AddCalls()[2].Event
	require.Equal(t, keptnv2.GetTriggeredEventType(keptnv2.EvaluationTaskName), verifyEvent.Event.Type())

	evaluationEvent := &keptnv2.EvaluationTriggeredEventData{}
	err = verifyEvent.Event.DataAs(evaluationEvent)
	require.Nil(t, err)
	require.Equal(t, 1, len(evaluationEvent.Deployment.DeploymentNames))
	require.Equal(t, "start", evaluationEvent.Test.Start)
	require.Equal(t, "end", evaluationEvent.Test.End)

	// STEP 6
	// send evaluation.started event
	sendAndVerifyStartedEvent(t, sc, keptnv2.EvaluationTaskName, triggeredID, "dev", "carts", "test-source")

	// STEP 7
	// send evaluation.finished event -> result = warning should not abort the task sequence
	triggeredID = sendAndVerifyFinishedEvent(t, sc, getEvaluationTaskFinishedEvent("dev", triggeredID, keptnv2.ResultWarning), keptnv2.EvaluationTaskName, keptnv2.ReleaseTaskName, "", "carts")
	require.Equal(t, 4, len(mockDispatcher.AddCalls()))
	verifyEvent = mockDispatcher.AddCalls()[3].Event
	require.Equal(t, keptnv2.GetTriggeredEventType(keptnv2.ReleaseTaskName), verifyEvent.Event.Type())

	// STEP 8
	// send release.started event
	sendAndVerifyStartedEvent(t, sc, keptnv2.ReleaseTaskName, triggeredID, "dev", "carts", "test-source")

	// STEP 9
	// send release.finished event
	triggeredID = sendAndVerifyFinishedEvent(t, sc, getReleaseTaskFinishedEvent("dev", triggeredID), keptnv2.ReleaseTaskName, keptnv2.DeploymentTaskName, "hardening", "carts")

	require.Equal(t, 7, len(mockDispatcher.AddCalls()))

	// verify dev.artifact-delivery.finished event
	sequenceFinishedEvent := mockDispatcher.AddCalls()[4].Event
	require.Equal(t, keptnv2.GetFinishedEventType("dev.artifact-delivery"), sequenceFinishedEvent.Event.Type())
	require.Equal(t, sequenceTriggeredEvent.ID, sequenceFinishedEvent.Event.Extensions()["triggeredid"])

	// verify hardening.artifact-delivery.triggered event
	nextSequenceTriggeredEvent := mockDispatcher.AddCalls()[5].Event
	require.Equal(t, keptnv2.GetTriggeredEventType("hardening.artifact-delivery"), nextSequenceTriggeredEvent.Event.Type())

	sequenceTriggeredDataMap := map[string]interface{}{}
	err = nextSequenceTriggeredEvent.Event.DataAs(&sequenceTriggeredDataMap)
	require.Nil(t, err)
	require.NotNil(t, sequenceTriggeredDataMap["configurationChange"])
	require.NotNil(t, sequenceTriggeredDataMap["deployment"])

	// verify deployment.triggered event for hardening stage
	verifyEvent = mockDispatcher.AddCalls()[6].Event
	require.Equal(t, keptnv2.GetTriggeredEventType(keptnv2.DeploymentTaskName), verifyEvent.Event.Type())

	// for a new stage the commit ID should be determined again since it is executed based on a different branch
	require.Equal(t, "latest-commit-id", verifyEvent.Event.Extensions()["gitcommitid"])
	deploymentEvent = &keptnv2.DeploymentTriggeredEventData{}
	err = verifyEvent.Event.DataAs(deploymentEvent)
	require.Nil(t, err)
	require.Equal(t, "hardening", deploymentEvent.Stage)
	require.Equal(t, "carts", deploymentEvent.ConfigurationChange.Values["image"])

	// verify that data from .finished events of the previous stage are included
	deploymentTriggeredDataMap := map[string]interface{}{}
	err = verifyEvent.Event.DataAs(&deploymentTriggeredDataMap)
	require.Nil(t, err)
	require.NotNil(t, deploymentTriggeredDataMap["test"])

	finishedEvents, _ := sc.eventRepo.GetEvents("test-project", common.EventFilter{
		Stage: common.Stringp("dev"),
	}, common.FinishedEvent)

	ShouldNotContainEvent(t, finishedEvents, keptnv2.GetFinishedEventType(keptnv2.DeploymentTaskName), "dev")
	ShouldNotContainEvent(t, finishedEvents, keptnv2.GetFinishedEventType(keptnv2.TestTaskName), "dev")
	ShouldNotContainEvent(t, finishedEvents, keptnv2.GetFinishedEventType(keptnv2.EvaluationTaskName), "dev")
	ShouldNotContainEvent(t, finishedEvents, keptnv2.GetFinishedEventType(keptnv2.ReleaseTaskName), "dev")

	// STEP 9.1
	// send deployment.started event 1 with ID 1
	sendAndVerifyStartedEvent(t, sc, keptnv2.DeploymentTaskName, triggeredID, "hardening", "carts", "test-source-1")

	// STEP 9.2
	// send deployment.started event 2 with ID 2
	sendAndVerifyStartedEvent(t, sc, keptnv2.DeploymentTaskName, triggeredID, "hardening", "carts", "test-source-2")

	// STEP 10.1
	// send deployment.finished event 1 with ID 1
	sendAndVerifyPartialFinishedEvent(t, sc, getDeploymentFinishedEvent("hardening", "carts", triggeredID, "test-source-1", keptnv2.ResultPass), keptnv2.DeploymentTaskName, keptnv2.ReleaseTaskName, "")
	// number of calls for dispatcher should not have increased before both finished events are received
	require.Equal(t, 7, len(mockDispatcher.AddCalls()))

	// STEP 10.2
	// send deployment.finished event 1 with ID 1
	triggeredID = sendAndVerifyFinishedEvent(t, sc, getDeploymentFinishedEvent("hardening", "carts", triggeredID, "test-source-2", keptnv2.ResultPass), keptnv2.DeploymentTaskName, keptnv2.TestTaskName, "", "carts")
	require.Equal(t, 8, len(mockDispatcher.AddCalls()))
	require.Equal(t, keptnv2.GetTriggeredEventType(keptnv2.TestTaskName), mockDispatcher.AddCalls()[7].Event.Event.Type())
}

func cleanupCollections(projectName string, sc *ShipyardController) {
	sc.sequenceExecutionRepo.Clear(projectName)
	sc.eventRepo.DeleteEventCollections(projectName)
	sc.projectMvRepo.DeleteProject(projectName)
}

//Scenario 2: Partial task sequence execution + triggering of next task sequence. Events are received out of order
func Test_shipyardController_Scenario2(t *testing.T) {
	t.Logf("Executing Shipyard Controller Scenario 2 with shipyard file %s", testShipyardFile)
	sc, cancel := getTestShipyardController("")

	defer cancel()
	defer cleanupCollections("test-project", sc)
	mockDispatcher := sc.eventDispatcher.(*fake.IEventDispatcherMock)

	// STEP 1
	// send dev.artifact-delivery.triggered event
	err := sc.HandleIncomingEvent(getArtifactDeliveryTriggeredEvent("dev", ""), true)
	if err != nil {
		t.Errorf("STEP 1 failed: HandleIncomingEvent(dev.artifact-delivery.triggered) returned %v", err)
		return
	}

	// check event dispatcher -> should contain deployment.triggered event with properties: [deployment]
	require.Equal(t, 1, len(mockDispatcher.AddCalls()))
	verifyEvent := mockDispatcher.AddCalls()[0].Event
	require.Equal(t, keptnv2.GetTriggeredEventType(keptnv2.DeploymentTaskName), verifyEvent.Event.Type())

	// check triggeredEvent Collection -> should contain deployment.triggered event
	triggeredEvents, _ := sc.eventRepo.GetEvents("test-project", common.EventFilter{
		Type:    keptnv2.GetTriggeredEventType(keptnv2.DeploymentTaskName),
		Stage:   common.Stringp("dev"),
		Service: common.Stringp("carts"),
		Source:  common.Stringp("shipyard-controller"),
	}, common.TriggeredEvent)
	ShouldContainEvent(t, triggeredEvents, keptnv2.GetTriggeredEventType(keptnv2.DeploymentTaskName), "", nil)
	triggeredID := triggeredEvents[0].ID

	// STEP 2
	// send deployment.started event
	go func() {
		<-time.After(2 * time.Second)
		sendAndVerifyStartedEvent(t, sc, keptnv2.DeploymentTaskName, triggeredID, "dev", "carts", "test-source")
	}()

	// STEP 3
	// send deployment.finished event

	err = sc.HandleIncomingEvent(getDeploymentFinishedEvent("dev", "carts", triggeredID, "test-source", keptnv2.ResultPass), true)
	require.Nil(t, err)

	require.Eventually(t, func() bool {
		return len(mockDispatcher.AddCalls()) == 2
	}, 10*time.Second, 1*time.Second)

	require.Equal(t, 2, len(mockDispatcher.AddCalls()))
	verifyEvent = mockDispatcher.AddCalls()[1].Event
	require.Equal(t, keptnv2.GetTriggeredEventType(keptnv2.TestTaskName), verifyEvent.Event.Type())

	taskEvent := &keptnv2.TestTriggeredEventData{}
	err = verifyEvent.Event.DataAs(taskEvent)
	require.Nil(t, err)
	require.Equal(t, 3, len(taskEvent.Deployment.DeploymentURIsPublic))
	require.Equal(t, 2, len(taskEvent.Deployment.DeploymentURIsLocal))

}

//Scenario 3: Received .finished event with status "errored" should abort task sequence and send .finished event with status "errored"
func Test_shipyardController_Scenario3(t *testing.T) {
	t.Logf("Executing Shipyard Controller Scenario 1 with shipyard file %s", testShipyardFile)
	sc, cancel := getTestShipyardController("")
	defer cleanupCollections("test-project", sc)
	defer cancel()

	mockDispatcher := sc.eventDispatcher.(*fake.IEventDispatcherMock)

	// STEP 1
	// send dev.artifact-delivery.triggered event
	err := sc.HandleIncomingEvent(getArtifactDeliveryTriggeredEvent("dev", ""), true)
	if err != nil {
		t.Errorf("STEP 1 failed: HandleIncomingEvent(dev.artifact-delivery.triggered) returned %v", err)
		return
	}

	// check event dispatcher -> should contain deployment.triggered event with properties: [deployment]
	require.Equal(t, 1, len(mockDispatcher.AddCalls()))
	verifyEvent := mockDispatcher.AddCalls()[0].Event
	require.Equal(t, keptnv2.GetTriggeredEventType(keptnv2.DeploymentTaskName), verifyEvent.Event.Type())

	// check triggeredEvent Collection -> should contain deployment.triggered event
	triggeredEvents, _ := sc.eventRepo.GetEvents("test-project", common.EventFilter{
		Type:    keptnv2.GetTriggeredEventType(keptnv2.DeploymentTaskName),
		Stage:   common.Stringp("dev"),
		Service: common.Stringp("carts"),
		Source:  common.Stringp("shipyard-controller"),
	}, common.TriggeredEvent)
	ShouldContainEvent(t, triggeredEvents, keptnv2.GetTriggeredEventType(keptnv2.DeploymentTaskName), "", nil)
	triggeredID := triggeredEvents[0].ID

	// STEP 2
	// send deployment.started event
	go func() {
		<-time.After(2 * time.Second)
		sendAndVerifyStartedEvent(t, sc, keptnv2.DeploymentTaskName, triggeredID, "dev", "carts", "test-source")
	}()

	// STEP 3
	// send deployment.finished event
	err = sc.HandleIncomingEvent(getErroredDeploymentFinishedEvent("dev", triggeredID, "test-source"), true)
	require.Nil(t, err)

	// check for dev.artifact-delivery.finished event
	require.Eventually(t, func() bool {
		return 4 == len(mockDispatcher.AddCalls())
	}, 10*time.Second, 1*time.Second)

	triggeredEvents, err = sc.eventRepo.GetEvents("test-project", common.EventFilter{
		Type:    keptnv2.GetTriggeredEventType(keptnv2.DeploymentTaskName),
		Stage:   common.Stringp("dev"),
		Service: common.Stringp("carts"),
		Source:  common.Stringp("shipyard-controller"),
	}, common.TriggeredEvent)

	require.Empty(t, triggeredEvents)
	taskSequenceCompletionEvent := mockDispatcher.AddCalls()[1].Event
	require.Equal(t, keptnv2.GetFinishedEventType("dev.artifact-delivery"), taskSequenceCompletionEvent.Event.Type())

	eventData := &keptnv2.EventData{}
	err = taskSequenceCompletionEvent.Event.DataAs(eventData)
	require.Nil(t, err)
	require.Equal(t, keptnv2.StatusErrored, eventData.Status)
	require.Equal(t, keptnv2.ResultFailed, eventData.Result)

	require.Equal(t, keptnv2.GetTriggeredEventType("dev.rollback"), mockDispatcher.AddCalls()[2].Event.Event.Type())
	require.Equal(t, keptnv2.GetTriggeredEventType("rollback"), mockDispatcher.AddCalls()[3].Event.Event.Type())

}

//Scenario 4: Received .finished event with result "fail" - stop task sequence
func Test_shipyardController_Scenario4(t *testing.T) {
	t.Logf("Executing Shipyard Controller Scenario 1 with shipyard file %s", testShipyardFile)
	sc, cancel := getTestShipyardController("")

	defer cancel()
	defer cleanupCollections("test-project", sc)

	mockDispatcher := sc.eventDispatcher.(*fake.IEventDispatcherMock)

	// STEP 1
	// send dev.artifact-delivery.triggered event
	err := sc.HandleIncomingEvent(getArtifactDeliveryTriggeredEvent("dev", ""), true)
	if err != nil {
		t.Errorf("STEP 1 failed: HandleIncomingEvent(dev.artifact-delivery.triggered) returned %v", err)
		return
	}

	// check event dispatcher -> should contain deployment.triggered event with properties: [deployment]
	require.Equal(t, 1, len(mockDispatcher.AddCalls()))
	verifyEvent := mockDispatcher.AddCalls()[0].Event
	require.Equal(t, keptnv2.GetTriggeredEventType(keptnv2.DeploymentTaskName), verifyEvent.Event.Type())

	// check triggeredEvent Collection -> should contain deployment.triggered event
	triggeredEvents, _ := sc.eventRepo.GetEvents("test-project", common.EventFilter{
		Type:    keptnv2.GetTriggeredEventType(keptnv2.DeploymentTaskName),
		Stage:   common.Stringp("dev"),
		Service: common.Stringp("carts"),
		Source:  common.Stringp("shipyard-controller"),
	}, common.TriggeredEvent)
	ShouldContainEvent(t, triggeredEvents, keptnv2.GetTriggeredEventType(keptnv2.DeploymentTaskName), "", nil)
	triggeredID := triggeredEvents[0].ID

	// STEP 2
	// send deployment.started event
	sendAndVerifyStartedEvent(t, sc, keptnv2.DeploymentTaskName, triggeredID, "dev", "carts", "test-source")

	// STEP 3
	// send deployment.finished event
	triggeredID = sendAndVerifyFinishedEvent(
		t,
		sc,
		getDeploymentFinishedEvent("dev", "carts", triggeredID, "test-source", keptnv2.ResultPass),
		keptnv2.DeploymentTaskName,
		keptnv2.TestTaskName,
		"",
		"carts",
	)

	require.Equal(t, 2, len(mockDispatcher.AddCalls()))
	verifyEvent = mockDispatcher.AddCalls()[1].Event
	require.Equal(t, keptnv2.GetTriggeredEventType(keptnv2.TestTaskName), verifyEvent.Event.Type())

	// STEP 4
	// send test.started event
	sendAndVerifyStartedEvent(t, sc, keptnv2.TestTaskName, triggeredID, "dev", "carts", "test-source")

	// STEP 5
	// send test.finished event
	triggeredID = sendAndVerifyFinishedEvent(
		t,
		sc,
		getTestTaskFinishedEvent("dev", triggeredID),
		keptnv2.TestTaskName,
		keptnv2.EvaluationTaskName,
		"",
		"carts",
	)

	require.Equal(t, 3, len(mockDispatcher.AddCalls()))
	verifyEvent = mockDispatcher.AddCalls()[2].Event
	require.Equal(t, keptnv2.GetTriggeredEventType(keptnv2.EvaluationTaskName), verifyEvent.Event.Type())
	// STEP 6
	// send evaluation.started event
	sendAndVerifyStartedEvent(t, sc, keptnv2.EvaluationTaskName, triggeredID, "dev", "carts", "test-source")

	// STEP 7
	// send evaluation.finished event with result=fail

	sendFinishedEventAndVerifyTaskSequenceCompletion(
		t,
		sc,
		getEvaluationTaskFinishedEvent("dev", triggeredID, keptnv2.ResultFailed),
		keptnv2.EvaluationTaskName,
		"",
	)

	// check for dev.artifact-delivery.finished event
	require.Equal(t, 6, len(mockDispatcher.AddCalls()))
	taskSequenceCompletionEvent := mockDispatcher.AddCalls()[3].Event
	require.Equal(t, keptnv2.GetFinishedEventType("dev.artifact-delivery"), taskSequenceCompletionEvent.Event.Type())

	eventData := &keptnv2.EventData{}
	err = taskSequenceCompletionEvent.Event.DataAs(eventData)
	require.Nil(t, err)
	require.Equal(t, keptnv2.StatusSucceeded, eventData.Status)
	require.Equal(t, keptnv2.ResultFailed, eventData.Result)

	require.Equal(t, keptnv2.GetTriggeredEventType("dev.rollback"), mockDispatcher.AddCalls()[4].Event.Event.Type())
	require.Equal(t, keptnv2.GetTriggeredEventType("rollback"), mockDispatcher.AddCalls()[5].Event.Event.Type())
}

//Scenario 4a: Handling multiple finished events, one has result==failed, ==> task sequence is stopped
func Test_shipyardController_Scenario4a(t *testing.T) {

	t.Logf("Executing Shipyard Controller Scenario 1 with shipyard file %s", testShipyardFile)
	sc, cancel := getTestShipyardController("")

	defer cancel()
	defer cleanupCollections("test-project", sc)

	mockDispatcher := sc.eventDispatcher.(*fake.IEventDispatcherMock)

	// STEP 1
	// send dev.artifact-delivery.triggered event
	err := sc.HandleIncomingEvent(getArtifactDeliveryTriggeredEvent("dev", ""), true)
	if err != nil {
		t.Errorf("STEP 1 failed: HandleIncomingEvent(dev.artifact-delivery.triggered) returned %v", err)
		return
	}

	// check event dispatcher -> should contain deployment.triggered event with properties: [deployment]
	require.Equal(t, 1, len(mockDispatcher.AddCalls()))
	verifyEvent := mockDispatcher.AddCalls()[0].Event
	require.Equal(t, keptnv2.GetTriggeredEventType(keptnv2.DeploymentTaskName), verifyEvent.Event.Type())

	// check triggeredEvent Collection -> should contain deployment.triggered event
	triggeredEvents, _ := sc.eventRepo.GetEvents("test-project", common.EventFilter{
		Type:    keptnv2.GetTriggeredEventType(keptnv2.DeploymentTaskName),
		Stage:   common.Stringp("dev"),
		Service: common.Stringp("carts"),
		Source:  common.Stringp("shipyard-controller"),
	}, common.TriggeredEvent)
	ShouldContainEvent(t, triggeredEvents, keptnv2.GetTriggeredEventType(keptnv2.DeploymentTaskName), "", nil)

	triggeredID := triggeredEvents[0].ID

	// STEP 2
	// send deployment.started events
	sendAndVerifyStartedEvent(t, sc, keptnv2.DeploymentTaskName, triggeredID, "dev", "carts", "test-source")

	sendAndVerifyStartedEvent(t, sc, keptnv2.DeploymentTaskName, triggeredID, "dev", "carts", "another-test-source")

	// STEP 3
	// send deployment.finished event
	err = sendFinishedEvent(sc, getDeploymentFinishedEvent("dev", "carts", triggeredID, "test-source", keptnv2.ResultFailed))
	require.Nil(t, err)

	sendFinishedEventAndVerifyTaskSequenceCompletion(
		t,
		sc,
		getDeploymentFinishedEvent("dev", "carts", triggeredID, "another-test-source", keptnv2.ResultPass),
		keptnv2.DeploymentTaskName,
		"",
	)

	// check for dev.artifact-delivery.finished event
	require.Equal(t, 4, len(mockDispatcher.AddCalls()))
	taskSequenceCompletionEvent := mockDispatcher.AddCalls()[1].Event
	require.Equal(t, keptnv2.GetFinishedEventType("dev.artifact-delivery"), taskSequenceCompletionEvent.Event.Type())

	eventData := &keptnv2.EventData{}
	err = taskSequenceCompletionEvent.Event.DataAs(eventData)
	require.Nil(t, err)
	require.Equal(t, keptnv2.StatusSucceeded, eventData.Status)
	require.Equal(t, keptnv2.ResultFailed, eventData.Result)

	require.Equal(t, keptnv2.GetTriggeredEventType("dev.rollback"), mockDispatcher.AddCalls()[2].Event.Event.Type())
	require.Equal(t, keptnv2.GetTriggeredEventType("rollback"), mockDispatcher.AddCalls()[3].Event.Event.Type())
}

//Scenario 4b: Received .finished event with result "fail" - stop task sequence and trigger next sequence based on result filter
func Test_shipyardController_TriggerOnFail(t *testing.T) {

	t.Logf("Executing Shipyard Controller with shipyard file %s", testShipyardFile)
	sc, cancel := getTestShipyardController("")

	defer cancel()
	defer cleanupCollections("test-project", sc)

	mockDispatcher := sc.eventDispatcher.(*fake.IEventDispatcherMock)

	// STEP 1
	// send dev.artifact-delivery.triggered event
	err := sc.HandleIncomingEvent(getArtifactDeliveryTriggeredEvent("dev", ""), true)
	if err != nil {
		t.Errorf("STEP 1 failed: HandleIncomingEvent(dev.artifact-delivery.triggered) returned %v", err)
		return
	}

	// check event dispatcher -> should contain deployment.triggered event with properties: [deployment]
	require.Equal(t, 1, len(mockDispatcher.AddCalls()))
	verifyEvent := mockDispatcher.AddCalls()[0].Event
	require.Equal(t, keptnv2.GetTriggeredEventType(keptnv2.DeploymentTaskName), verifyEvent.Event.Type())

	// check triggeredEvent Collection -> should contain deployment.triggered event
	triggeredEvents, _ := sc.eventRepo.GetEvents("test-project", common.EventFilter{
		Type:    keptnv2.GetTriggeredEventType(keptnv2.DeploymentTaskName),
		Stage:   common.Stringp("dev"),
		Service: common.Stringp("carts"),
		Source:  common.Stringp("shipyard-controller"),
	}, common.TriggeredEvent)
	ShouldContainEvent(t, triggeredEvents, keptnv2.GetTriggeredEventType(keptnv2.DeploymentTaskName), "", nil)

	triggeredID := triggeredEvents[0].ID

	// STEP 2
	// send deployment.started event
	sendAndVerifyStartedEvent(t, sc, keptnv2.DeploymentTaskName, triggeredID, "dev", "carts", "test-source")

	// STEP 3
	// send deployment.finished event
	sendFinishedEventAndVerifyTaskSequenceCompletion(
		t,
		sc,
		getDeploymentFinishedEvent("dev", "carts", triggeredID, "test-source", keptnv2.ResultFailed),
		keptnv2.DeploymentTaskName,
		"",
	)

	// check for dev.artifact-delivery.finished event
	require.Equal(t, 4, len(mockDispatcher.AddCalls()))
	taskSequenceCompletionEvent := mockDispatcher.AddCalls()[1].Event
	require.Equal(t, keptnv2.GetFinishedEventType("dev.artifact-delivery"), taskSequenceCompletionEvent.Event.Type())

	eventData := &keptnv2.EventData{}
	err = taskSequenceCompletionEvent.Event.DataAs(eventData)
	require.Nil(t, err)
	require.Equal(t, keptnv2.StatusSucceeded, eventData.Status)
	require.Equal(t, keptnv2.ResultFailed, eventData.Result)

	// check for dev.rollback.triggered
	rollbackTriggeredEvent := mockDispatcher.AddCalls()[2].Event
	require.Equal(t, keptnv2.GetTriggeredEventType("dev.rollback"), rollbackTriggeredEvent.Event.Type())

	// check for rollback.triggered
	rollbackTaskTriggeredEvent := mockDispatcher.AddCalls()[3].Event
	require.Equal(t, keptnv2.GetTriggeredEventType(keptnv2.RollbackTaskName), rollbackTaskTriggeredEvent.Event.Type())

	for _, addCall := range mockDispatcher.AddCalls() {
		// hardening.artifact-delivery should not be triggered
		require.NotEqual(t, keptnv2.GetTriggeredEventType("hardening.artifact-delivery"), addCall.Event.Event.Type())
	}
}

//Scenario 5: Received .triggered event for project with invalid shipyard version -> send .finished event with result = fail
func Test_shipyardController_Scenario5(t *testing.T) {
	t.Logf("Executing Shipyard Controller Scenario 5 with shipyard file %s", testShipyardFileWithInvalidVersion)
	sc, cancel := getTestShipyardController(testShipyardFileWithInvalidVersion)

	defer cancel()
	defer cleanupCollections("test-project", sc)

	mockDispatcher := sc.eventDispatcher.(*fake.IEventDispatcherMock)

	// STEP 1
	// send dev.artifact-delivery.triggered event
	err := sc.HandleIncomingEvent(getArtifactDeliveryTriggeredEvent("dev", ""), true)
	if err != nil {
		t.Errorf("STEP 1 failed: HandleIncomingEvent(dev.artifact-delivery.triggered) returned %v", err)
		return
	}

	require.Equal(t, 1, len(mockDispatcher.AddCalls()))
	verifyEvent := mockDispatcher.AddCalls()[0].Event
	require.Equal(t, keptnv2.GetFinishedEventType("dev.artifact-delivery"), verifyEvent.Event.Type())

}

//Scenario 6: Received .finished event for a task where no sequence is available
func Test_shipyardController_Scenario6(t *testing.T) {
	t.Logf("Executing Shipyard Controller Scenario 6 with shipyard file %s", testShipyardFileWithInvalidVersion)
	sc, cancel := getTestShipyardController(testShipyardFileWithInvalidVersion)

	defer cancel()
	defer cleanupCollections("test-project", sc)

	mockDispatcher := sc.eventDispatcher.(*fake.IEventDispatcherMock)

	// STEP 1
	// send dev.artifact-delivery.triggered event
	err := sc.HandleIncomingEvent(getTestTaskFinishedEvent("dev", "unknown-triggered-id"), true)
	require.NotNil(t, err)
	require.ErrorIs(t, err, common.ErrSequenceNotFound)

	require.Empty(t, mockDispatcher.AddCalls())
}

//Scenario 7: Received .finished event with missing stage
func Test_shipyardController_Scenario7(t *testing.T) {
	t.Logf("Executing Shipyard Controller Scenario 7 with shipyard file %s", testShipyardFileWithInvalidVersion)
	sc, cancel := getTestShipyardController(testShipyardFileWithInvalidVersion)

	defer cancel()
	defer cleanupCollections("test-project", sc)

	mockDispatcher := sc.eventDispatcher.(*fake.IEventDispatcherMock)

	// STEP 1
	// send dev.artifact-delivery.triggered event
	err := sc.HandleIncomingEvent(getTestTaskFinishedEvent("", "unknown-triggered-id"), true)
	require.NotNil(t, err)
	require.ErrorIs(t, err, models.ErrInvalidEventScope)

	require.Empty(t, mockDispatcher.AddCalls())
}

//Scenario 8: Two complete task sequence execution same stage different service do not interfere. Third execution same service fails.
func Test_shipyardController_Scenario8(t *testing.T) {
	sequence_execution_queue := []models.DispatcherEvent{}
	t.Logf("Executing Shipyard Controller Scenario 8 with shipyard file %s", testShipyardFile)
	sc, cancel := getTestShipyardController("")
	defer sc.StopDispatchers()
	defer cancel()
	defer sc.sequenceExecutionRepo.Clear("test-project")

	mockDispatcher := sc.eventDispatcher.(*fake.IEventDispatcherMock)
	mockDispatcher.AddFunc = func(event models.DispatcherEvent, skipQueue bool) error {
		e := EventScope{}
		event.Event.DataAs(&e)
		for _, s := range sequence_execution_queue {
			eventData := keptnv2.EventData{}
			s.Event.DataAs(&eventData)

			if eventData.Service == e.Service && event.Event.Type() == s.Event.Type() {
				return errors.New("cannot send event with same service ")
			}
		}
		sequence_execution_queue = append(sequence_execution_queue, event)
		return nil
	}
	commitID := "my-commit-id"
	commitID2 := "my-commit-id2"
	// STEP 1
	// send dev.artifact-delivery.triggered event
	sequenceTriggeredEvent := getArtifactDeliveryTriggeredEvent("dev", commitID)
	err := sc.HandleIncomingEvent(sequenceTriggeredEvent, true)
	if err != nil {
		t.Errorf("STEP 1 failed: HandleIncomingEvent(dev.artifact-delivery.triggered) returned %v", err)
		return
	}

	// check event dispatcher -> should contain deployment.triggered event with properties: [deployment]
	time.Sleep(3 * time.Second) //wait for dispatching
	require.Equal(t, 1, len(mockDispatcher.AddCalls()))
	verifyEvent := mockDispatcher.AddCalls()[0].Event
	require.Equal(t, keptnv2.GetTriggeredEventType(keptnv2.DeploymentTaskName), verifyEvent.Event.Type())

	deploymentEvent := &keptnv2.DeploymentTriggeredEventData{}
	err = verifyEvent.Event.DataAs(deploymentEvent)
	require.Nil(t, err)
	require.Equal(t, 1, len(deploymentEvent.Deployment.DeploymentURIsPublic))
	require.Equal(t, "direct", deploymentEvent.Deployment.DeploymentStrategy)
	require.Equal(t, "carts", deploymentEvent.ConfigurationChange.Values["image"])

	// check triggeredEvent Collection -> should contain deployment.triggered event
	triggeredEvents, _ := sc.eventRepo.GetEvents("test-project", common.EventFilter{
		Type:    keptnv2.GetTriggeredEventType(keptnv2.DeploymentTaskName),
		Stage:   common.Stringp("dev"),
		Service: common.Stringp("carts"),
		Source:  common.Stringp("shipyard-controller"),
	}, common.TriggeredEvent)
	ShouldContainEvent(t, triggeredEvents, keptnv2.GetTriggeredEventType(keptnv2.DeploymentTaskName), "", nil)

	require.Equal(t, commitID, triggeredEvents[0].GitCommitID)
	triggeredID := triggeredEvents[0].ID

	// STEP 2
	// send deployment.started event
	sendAndVerifyStartedEvent(t, sc, keptnv2.DeploymentTaskName, triggeredID, "dev", "carts", "test-source")

	// STEP 1.b
	// send dev.artifact-delivery.triggered event
	sequenceTriggeredEvent2 := getArtifactDeliveryCarts2TriggeredEvent("dev", commitID2)
	err = sc.HandleIncomingEvent(sequenceTriggeredEvent2, true)
	if err != nil {
		t.Errorf("STEP 1.b failed: HandleIncomingEvent(dev.artifact-delivery.triggered) returned %v", err)
		return
	}

	// check event dispatcher -> should contain deployment.triggered event with properties: [deployment]
	time.Sleep(3 * time.Second) //wait for dispatching
	require.Equal(t, 2, len(mockDispatcher.AddCalls()))
	verifyEvent = mockDispatcher.AddCalls()[1].Event
	require.Equal(t, keptnv2.GetTriggeredEventType(keptnv2.DeploymentTaskName), verifyEvent.Event.Type())

	deploymentEvent = &keptnv2.DeploymentTriggeredEventData{}
	err = verifyEvent.Event.DataAs(deploymentEvent)
	require.Nil(t, err)
	require.Equal(t, 1, len(deploymentEvent.Deployment.DeploymentURIsPublic))
	require.Equal(t, "direct", deploymentEvent.Deployment.DeploymentStrategy)
	require.Equal(t, "carts2", deploymentEvent.ConfigurationChange.Values["image"])

	// check triggeredEvent Collection -> should contain deployment.triggered event
	triggeredEvents, _ = sc.eventRepo.GetEvents("test-project", common.EventFilter{
		Type:    keptnv2.GetTriggeredEventType(keptnv2.DeploymentTaskName),
		Stage:   common.Stringp("dev"),
		Service: common.Stringp("carts2"),
		Source:  common.Stringp("shipyard-controller"),
	}, common.TriggeredEvent)
	ShouldContainEvent(t, triggeredEvents, keptnv2.GetTriggeredEventType(keptnv2.DeploymentTaskName), "", nil)

	triggeredID2 := triggeredEvents[0].ID
	require.Equal(t, commitID2, triggeredEvents[0].GitCommitID)
	// STEP 2.b
	// send deployment.started event
	sendAndVerifyStartedEvent(t, sc, keptnv2.DeploymentTaskName, triggeredID2, "dev", "carts2", "test-source")
	time.Sleep(3 * time.Second) //wait for dispatching

	// STEP 1.c
	// send dev.artifact-delivery.triggered event again for a carts deployment
	sequenceTriggeredEvent3 := getArtifactDeliveryTriggeredEventCarts2("dev", commitID2)
	err = sc.HandleIncomingEvent(sequenceTriggeredEvent3, true)
	if err == nil {
		t.Error("STEP 1.c failed: no error returned!")
		return
	}
	t.Logf("STEP 1.c HandleIncomingEvent(dev.artifact-delivery.triggered) returned %v", err)

	// STEP 3
	// send deployment.finished event
	triggeredID = sendAndVerifyFinishedEvent(
		t,
		sc,
		getDeploymentFinishedEvent("dev", "carts", triggeredID, "test-source", keptnv2.ResultPass),
		keptnv2.DeploymentTaskName,
		keptnv2.TestTaskName,
		"",
		"carts",
	)

	// STEP 3.b
	// send deployment.finished event
	triggeredID2 = sendAndVerifyFinishedEvent(
		t,
		sc,
		getDeploymentFinishedEvent("dev", "carts2", triggeredID2, "test-source", keptnv2.ResultPass),
		keptnv2.DeploymentTaskName,
		keptnv2.TestTaskName,
		"",
		"carts2",
	)

}

func Test_shipyardController_DuplicateTask(t *testing.T) {
	t.Logf("Executing Shipyard Controller Scenario 6 (duplicate tasks) with shipyard file %s", testShipyardFileWithDuplicateTasks)
	sc, cancel := getTestShipyardController(testShipyardFileWithDuplicateTasks)
	defer cancel()
	mockDispatcher := sc.eventDispatcher.(*fake.IEventDispatcherMock)

	// STEP 1
	// send dev.artifact-delivery.triggered event
	err := sc.HandleIncomingEvent(getArtifactDeliveryTriggeredEvent("dev", ""), true)
	if err != nil {
		t.Errorf("STEP 1 failed: HandleIncomingEvent(dev.artifact-delivery.triggered) returned %v", err)
		return
	}

	require.Equal(t, 1, len(mockDispatcher.AddCalls()))
	triggeredEvent := mockDispatcher.AddCalls()[0].Event
	triggeredKeptnEvent, err := keptnv2.ToKeptnEvent(triggeredEvent.Event)
	require.Equal(t, keptnv2.GetTriggeredEventType(keptnv2.DeploymentTaskName), *triggeredKeptnEvent.Type)

	// STEP 2
	// send deployment.started event
	sendAndVerifyStartedEvent(t, sc, keptnv2.DeploymentTaskName, triggeredKeptnEvent.ID, "dev", "carts", "test-source")

	// STEP 3
	// send deployment.finished event
	triggeredID := sendAndVerifyFinishedEvent(
		t,
		sc,
		getDeploymentFinishedEvent("dev", "carts", triggeredKeptnEvent.ID, "test-source", keptnv2.ResultPass),
		keptnv2.DeploymentTaskName,
		keptnv2.DeploymentTaskName,
		"",
		"carts",
	)

	// STEP 4
	// send deployment.started event (for the second deployment task)
	sendAndVerifyStartedEvent(t, sc, keptnv2.DeploymentTaskName, triggeredID, "dev", "carts", "test-source")

	// STEP 5
	// send deployment.finished event for the second deployment task -> now we want an evaluation.triggered event as the next task
	triggeredID = sendAndVerifyFinishedEvent(
		t,
		sc,
		getDeploymentFinishedEvent("dev", "carts", triggeredID, "test-source", keptnv2.ResultPass),
		keptnv2.DeploymentTaskName,
		keptnv2.EvaluationTaskName,
		"",
		"carts",
	)
}

func Test_shipyardController_TimeoutSequence(t *testing.T) {
	sc, cancel := getTestShipyardController("")
	defer cancel()
	fakeTimeoutHook := &fake.ISequenceTimeoutHookMock{OnSequenceTimeoutFunc: func(event apimodels.KeptnContextExtendedCE) {}}
	sc.AddSequenceTimeoutHook(fakeTimeoutHook)

	// insert the test data
	_ = sc.eventRepo.InsertEvent("my-project", apimodels.KeptnContextExtendedCE{
		Data: keptnv2.EventData{
			Project: "my-project",
			Stage:   "my-stage",
			Service: "my-service",
		},
		ID:             "my-sequence-triggered-id",
		Shkeptncontext: "my-keptn-context-id",
		Type:           common.Stringp(keptnv2.GetTriggeredEventType("my-stage.delivery")),
	}, common.TriggeredEvent)

	_ = sc.eventRepo.InsertEvent("my-project", apimodels.KeptnContextExtendedCE{
		Data: keptnv2.EventData{
			Project: "my-project",
			Stage:   "my-stage",
			Service: "my-service",
		},
		ID:             "my-deployment-triggered-id",
		Shkeptncontext: "my-keptn-context-id",
		Type:           common.Stringp(keptnv2.GetTriggeredEventType(keptnv2.DeploymentTaskName)),
	}, common.TriggeredEvent)

	err := sc.sequenceExecutionRepo.Upsert(models.SequenceExecution{
		ID: "sequence-execution-id",
		Sequence: keptnv2.Sequence{
			Name: "delivery",
		},
		Status: models.SequenceExecutionStatus{
			State: apimodels.SequenceStartedState,
			CurrentTask: models.TaskExecutionState{
				Name:        "deployment",
				TriggeredID: "my-deployment-triggered-id",
			},
		},
		Scope: models.EventScope{
			KeptnContext: "my-keptn-context-id",
			EventData: keptnv2.EventData{
				Project: "my-project",
				Stage:   "my-stage",
				Service: "my-service",
			},
		},
	}, nil)

	require.Nil(t, err)

	// invoke the CancelSequence function
	err = sc.timeoutSequence(apimodels.SequenceTimeout{
		KeptnContext: "my-keptn-context-id",
		LastEvent: apimodels.KeptnContextExtendedCE{
			Data: keptnv2.EventData{
				Project: "my-project",
				Stage:   "my-stage",
				Service: "my-service",
			},
			Type:           common.Stringp(keptnv2.GetTriggeredEventType("my-task")),
			ID:             "my-deployment-triggered-id",
			Shkeptncontext: "my-keptn-context-id",
		},
	})

	require.Nil(t, err)
	require.Len(t, fakeTimeoutHook.OnSequenceTimeoutCalls(), 1)

	eventDispatcherMock := sc.eventDispatcher.(*fake.IEventDispatcherMock)

	require.Len(t, eventDispatcherMock.AddCalls(), 1)

	sentEvent := eventDispatcherMock.AddCalls()[0]

	eventData := &keptnv2.EventData{}
	err = sentEvent.Event.Event.DataAs(eventData)

	require.Nil(t, err)
	require.Equal(t, keptnv2.ResultFailed, eventData.Result)
	require.Equal(t, keptnv2.StatusErrored, eventData.Status)
}

func Test_shipyardController_TimeoutSequence_ErrorWhenSendingEvent(t *testing.T) {
	sc, cancel := getTestShipyardController("")
	defer cancel()
	fakeTimeoutHook := &fake.ISequenceTimeoutHookMock{OnSequenceTimeoutFunc: func(event apimodels.KeptnContextExtendedCE) {}}
	sc.AddSequenceTimeoutHook(fakeTimeoutHook)

	// insert the test data
	_ = sc.eventRepo.InsertEvent("my-project", apimodels.KeptnContextExtendedCE{
		Data: keptnv2.EventData{
			Project: "my-project",
			Stage:   "my-stage",
			Service: "my-service",
		},
		ID:             "my-sequence-triggered-id",
		Shkeptncontext: "my-keptn-context-id",
		Type:           common.Stringp(keptnv2.GetTriggeredEventType("my-stage.delivery")),
	}, common.TriggeredEvent)

	_ = sc.eventRepo.InsertEvent("my-project", apimodels.KeptnContextExtendedCE{
		Data: keptnv2.EventData{
			Project: "my-project",
			Stage:   "my-stage",
			Service: "my-service",
		},
		ID:             "my-deployment-triggered-id",
		Shkeptncontext: "my-keptn-context-id",
		Type:           common.Stringp(keptnv2.GetTriggeredEventType(keptnv2.DeploymentTaskName)),
	}, common.TriggeredEvent)

	err := sc.sequenceExecutionRepo.Upsert(models.SequenceExecution{
		ID: "sequence-execution-id",
		Sequence: keptnv2.Sequence{
			Name: "delivery",
		},
		Status: models.SequenceExecutionStatus{
			State: apimodels.SequenceStartedState,
			CurrentTask: models.TaskExecutionState{
				Name:        "deployment",
				TriggeredID: "my-deployment-triggered-id",
			},
		},
		Scope: models.EventScope{
			KeptnContext: "my-keptn-context-id",
			EventData: keptnv2.EventData{
				Project: "my-project",
				Stage:   "my-stage",
				Service: "my-service",
			},
		},
	}, nil)

	require.Nil(t, err)

	eventDispatcherMock := sc.eventDispatcher.(*fake.IEventDispatcherMock)
	eventDispatcherMock.AddFunc = func(event models.DispatcherEvent, skipQueue bool) error {
		return errors.New("oops")
	}

	// invoke the CancelSequence function
	err = sc.timeoutSequence(apimodels.SequenceTimeout{
		KeptnContext: "my-keptn-context-id",
		LastEvent: apimodels.KeptnContextExtendedCE{
			Data: keptnv2.EventData{
				Project: "my-project",
				Stage:   "my-stage",
				Service: "my-service",
			},
			Type:           common.Stringp(keptnv2.GetTriggeredEventType("my-task")),
			ID:             "my-deployment-triggered-id",
			Shkeptncontext: "my-keptn-context-id",
		},
	})

	require.NotNil(t, err)
	require.Len(t, fakeTimeoutHook.OnSequenceTimeoutCalls(), 1)

	require.Len(t, eventDispatcherMock.AddCalls(), 1)

	sentEvent := eventDispatcherMock.AddCalls()[0]

	eventData := &keptnv2.EventData{}
	err = sentEvent.Event.Event.DataAs(eventData)

	require.Nil(t, err)
	require.Equal(t, keptnv2.ResultFailed, eventData.Result)
	require.Equal(t, keptnv2.StatusErrored, eventData.Status)
}

func Test_shipyardController_CancelSequence(t *testing.T) {
	sc, cancel := getTestShipyardController("")
	defer cancel()
	fakeSequenceFinishedHook := &fake.ISequenceFinishedHookMock{OnSequenceFinishedFunc: func(event apimodels.KeptnContextExtendedCE) {}}
	sc.AddSequenceFinishedHook(fakeSequenceFinishedHook)

	fakeSequenceAbortedHook := &fake.ISequenceAbortedHookMock{OnSequenceAbortedFunc: func(eventScope models.EventScope) {}}
	sc.AddSequenceAbortedHook(fakeSequenceAbortedHook)

	// insert the test data
	_ = sc.eventRepo.InsertEvent("my-project", apimodels.KeptnContextExtendedCE{
		Data: keptnv2.EventData{
			Project: "my-project",
			Stage:   "my-stage",
			Service: "my-service",
		},
		ID:             "my-sequence-triggered-id",
		Shkeptncontext: "my-keptn-context-id",
		Type:           common.Stringp(keptnv2.GetTriggeredEventType("my-stage.delivery")),
	}, common.TriggeredEvent)

	_ = sc.eventRepo.InsertEvent("my-project", apimodels.KeptnContextExtendedCE{
		Data: keptnv2.EventData{
			Project: "my-project",
			Stage:   "my-stage",
			Service: "my-service",
		},
		ID:             "my-deployment-triggered-id",
		Shkeptncontext: "my-keptn-context-id",
		Type:           common.Stringp(keptnv2.GetTriggeredEventType(keptnv2.DeploymentTaskName)),
	}, common.TriggeredEvent)

	err := sc.sequenceExecutionRepo.Upsert(models.SequenceExecution{
		ID: "sequence-execution-id",
		Sequence: keptnv2.Sequence{
			Name: "delivery",
		},
		Status: models.SequenceExecutionStatus{
			State: apimodels.SequenceStartedState,
			CurrentTask: models.TaskExecutionState{
				Name:        "deployment",
				TriggeredID: "my-deployment-triggered-id",
			},
		},
		Scope: models.EventScope{
			KeptnContext: "my-keptn-context-id",
			EventData: keptnv2.EventData{
				Project: "my-project",
				Stage:   "my-stage",
				Service: "my-service",
			},
		},
	}, nil)

	require.Nil(t, err)

	// invoke the CancelSequence function
	err = sc.cancelSequence(apimodels.SequenceControl{
		KeptnContext: "my-keptn-context-id",
		Project:      "my-project",
		Stage:        "my-stage",
	})

	require.Nil(t, err)
	require.Len(t, fakeSequenceFinishedHook.OnSequenceFinishedCalls(), 0)
	require.Len(t, fakeSequenceAbortedHook.OnSequenceAbortedCalls(), 1)
}

func Test_shipyardController_CancelQueuedSequence(t *testing.T) {
	sc, cancel := getTestShipyardController("")
	defer cancel()
	sequenceDispatcherMock := &fake.ISequenceDispatcherMock{}
	sequenceDispatcherMock.RemoveFunc = func(eventScope models.EventScope) error {
		return nil
	}

	sc.sequenceDispatcher = sequenceDispatcherMock

	fakeSequenceFinishedHook := &fake.ISequenceFinishedHookMock{OnSequenceFinishedFunc: func(event apimodels.KeptnContextExtendedCE) {}}
	sc.AddSequenceFinishedHook(fakeSequenceFinishedHook)

	fakeSequenceAbortedHook := &fake.ISequenceAbortedHookMock{OnSequenceAbortedFunc: func(eventScope models.EventScope) {}}
	sc.AddSequenceAbortedHook(fakeSequenceAbortedHook)

	// insert the test data
	_ = sc.eventRepo.InsertEvent("my-project", apimodels.KeptnContextExtendedCE{
		Data: keptnv2.EventData{
			Project: "my-project",
			Stage:   "my-stage",
			Service: "my-service",
		},
		ID:             "my-sequence-triggered-id",
		Shkeptncontext: "my-keptn-context-id",
		Type:           common.Stringp(keptnv2.GetTriggeredEventType("my-stage.delivery")),
	}, common.TriggeredEvent)

	err := sc.sequenceExecutionRepo.Upsert(models.SequenceExecution{
		ID: "sequence-execution-id",
		Sequence: keptnv2.Sequence{
			Name: "delivery",
		},
		Status: models.SequenceExecutionStatus{
			State: apimodels.SequenceTriggeredState,
		},
		Scope: models.EventScope{
			KeptnContext: "my-keptn-context-id",
			EventData: keptnv2.EventData{
				Project: "my-project",
				Stage:   "my-stage",
				Service: "my-service",
			},
		},
	}, nil)

	require.Nil(t, err)

	// invoke the CancelSequence function
	err = sc.cancelSequence(apimodels.SequenceControl{
		KeptnContext: "my-keptn-context-id",
		Project:      "my-project",
		Stage:        "my-stage",
	})

	require.Nil(t, err)
	require.Len(t, fakeSequenceFinishedHook.OnSequenceFinishedCalls(), 0)
	require.Len(t, fakeSequenceAbortedHook.OnSequenceAbortedCalls(), 1)
}

func Test_shipyardController_CancelQueuedSequence_RemoveFromQueueFails(t *testing.T) {
	sc, cancel := getTestShipyardController("")
	defer cancel()
	sequenceDispatcherMock := &fake.ISequenceDispatcherMock{}
	sequenceDispatcherMock.RemoveFunc = func(eventScope models.EventScope) error {
		return errors.New("oops")
	}

	sc.sequenceDispatcher = sequenceDispatcherMock

	fakeSequenceFinishedHook := &fake.ISequenceFinishedHookMock{OnSequenceFinishedFunc: func(event apimodels.KeptnContextExtendedCE) {}}
	sc.AddSequenceFinishedHook(fakeSequenceFinishedHook)

	fakeSequenceAbortedHook := &fake.ISequenceAbortedHookMock{OnSequenceAbortedFunc: func(eventScope models.EventScope) {}}
	sc.AddSequenceAbortedHook(fakeSequenceAbortedHook)

	// insert the test data
	_ = sc.eventRepo.InsertEvent("my-project", apimodels.KeptnContextExtendedCE{
		Data: keptnv2.EventData{
			Project: "my-project",
			Stage:   "my-stage",
			Service: "my-service",
		},
		ID:             "my-sequence-triggered-id",
		Shkeptncontext: "my-keptn-context-id",
		Type:           common.Stringp(keptnv2.GetTriggeredEventType("my-stage.delivery")),
	}, common.TriggeredEvent)

	err := sc.sequenceExecutionRepo.Upsert(models.SequenceExecution{
		ID: "sequence-execution-id",
		Sequence: keptnv2.Sequence{
			Name: "delivery",
		},
		Status: models.SequenceExecutionStatus{
			State: apimodels.SequenceTriggeredState,
		},
		Scope: models.EventScope{
			KeptnContext: "my-keptn-context-id",
			EventData: keptnv2.EventData{
				Project: "my-project",
				Stage:   "my-stage",
				Service: "my-service",
			},
		},
	}, nil)

	// invoke the CancelSequence function
	err = sc.cancelSequence(apimodels.SequenceControl{
		KeptnContext: "my-keptn-context-id",
		Project:      "my-project",
		Stage:        "my-stage",
	})

	require.Nil(t, err)
	require.Len(t, fakeSequenceFinishedHook.OnSequenceFinishedCalls(), 0)
	require.Len(t, fakeSequenceAbortedHook.OnSequenceAbortedCalls(), 1)
}

func Test_shipyardController_CancelQueuedSequence_NoTriggeredEventAvailable(t *testing.T) {
	sc, cancel := getTestShipyardController("")
	defer cancel()
	sequenceDispatcherMock := &fake.ISequenceDispatcherMock{}
	sequenceDispatcherMock.RemoveFunc = func(eventScope models.EventScope) error {
		return nil
	}

	sc.sequenceDispatcher = sequenceDispatcherMock

	fakeSequenceFinishedHook := &fake.ISequenceFinishedHookMock{OnSequenceFinishedFunc: func(event apimodels.KeptnContextExtendedCE) {}}
	sc.AddSequenceFinishedHook(fakeSequenceFinishedHook)

	fakeSequenceAbortedHook := &fake.ISequenceAbortedHookMock{OnSequenceAbortedFunc: func(eventScope models.EventScope) {}}
	sc.AddSequenceAbortedHook(fakeSequenceAbortedHook)

	// invoke the CancelSequence function
	err := sc.cancelSequence(apimodels.SequenceControl{
		KeptnContext: "my-keptn-context-id",
		Project:      "my-project",
		Stage:        "my-stage",
	})

	require.Nil(t, err)
	require.Len(t, fakeSequenceFinishedHook.OnSequenceFinishedCalls(), 0)
	require.Len(t, fakeSequenceAbortedHook.OnSequenceAbortedCalls(), 1)
}

func Test_SequenceForUnavailableStage(t *testing.T) {
	t.Logf("Executing Shipyard Controller with shipyard file %s", testShipyardFile)
	sc, cancel := getTestShipyardController("")
	defer cancel()
	sc.sequenceDispatcher = &fake.ISequenceDispatcherMock{
		AddFunc: func(queueItem models.QueueItem) error {
			return nil
		},
	}

	mockEventDispatcher := sc.eventDispatcher.(*fake.IEventDispatcherMock)
	mockSequenceDispatcher := sc.sequenceDispatcher.(*fake.ISequenceDispatcherMock)

	// STEP 1
	// send unknown.artifact-delivery.triggered event
	err := sc.HandleIncomingEvent(getArtifactDeliveryTriggeredEvent("unknown", ""), true)

	require.Nil(t, err)
	require.Len(t, mockEventDispatcher.AddCalls(), 1)
	require.Equal(t, keptnv2.GetFinishedEventType("unknown.artifact-delivery"), mockEventDispatcher.AddCalls()[0].Event.Event.Type())
	require.Empty(t, mockSequenceDispatcher.AddCalls())
}

// Updating event of service fails -> event handling should still happen
func Test_UpdateEventOfServiceFailsFails(t *testing.T) {
	t.Logf("Executing Shipyard Controller with shipyard file %s", testShipyardFileWithInvalidVersion)
	sc, cancel := getTestShipyardController(testShipyardFileWithInvalidVersion)
	defer cancel()
	mockDispatcher := sc.eventDispatcher.(*fake.IEventDispatcherMock)

	// STEP 1
	// send dev.artifact-delivery.triggered event
	err := sc.HandleIncomingEvent(getArtifactDeliveryTriggeredEvent("dev", ""), true)
	if err != nil {
		t.Errorf("STEP 1 failed: HandleIncomingEvent(dev.artifact-delivery.triggered) returned %v", err)
		return
	}

	require.Eventually(t, func() bool {
		return len(mockDispatcher.AddCalls()) == 1
	}, 10*time.Second, 1*time.Second)
	require.Equal(t, 1, len(mockDispatcher.AddCalls()))
	verifyEvent := mockDispatcher.AddCalls()[0].Event
	require.Equal(t, keptnv2.GetFinishedEventType("dev.artifact-delivery"), verifyEvent.Event.Type())
}

// Scenario 5: Received .triggered event for project with invalid shipyard version -> send .finished event with result = fail
func Test_UpdateServiceShouldNotBeCalledForEmptyService(t *testing.T) {
	t.Logf("Executing Shipyard Controller with shipyard file %s", testShipyardFileWithInvalidVersion)
	sc, cancel := getTestShipyardController("")

	defer cancel()
	event := getArtifactDeliveryTriggeredEvent("dev", "")

	event.Data = keptnv2.EventData{
		Project: "my-project",
		Stage:   "my-stage",
		Service: "",
	}
	// STEP 1
	// send dev.artifact-delivery.triggered event
	err := sc.HandleIncomingEvent(event, true)

	assert.NotNil(t, err)
}

func getTestShipyardController(shipyardContent string) (*ShipyardController, context.CancelFunc) {
	if shipyardContent == "" {
		shipyardContent = testShipyardFile
	}
	os.Setenv("DISABLE_LEADER_ELECTION", "true")

	eventRepo := db.NewMongoDBEventsRepo(db.GetMongoDBConnectionInstance())
	sequenceQueueRepo := db.NewMongoDBSequenceQueueRepo(db.GetMongoDBConnectionInstance())
	sequenceExecutionRepo := db.NewMongoDBSequenceExecutionRepo(db.GetMongoDBConnectionInstance())
	sequenceDispatcher := NewSequenceDispatcher(
		eventRepo,
		sequenceQueueRepo,
		sequenceExecutionRepo,
		time.Second,
		clock.New(),
		common.SDModeRW,
	)
	sc := &ShipyardController{
		projectMvRepo: db.NewProjectMVRepo(db.NewMongoDBKeyEncodingProjectsRepo(db.GetMongoDBConnectionInstance()), db.NewMongoDBEventsRepo(db.GetMongoDBConnectionInstance()), db.NewMongoDBSequenceExecutionRepo(db.GetMongoDBConnectionInstance())),
		eventRepo:     eventRepo,
		eventDispatcher: &fake.IEventDispatcherMock{
			AddFunc: func(event models.DispatcherEvent, skipQueue bool) error {
				return nil
			},
			RunFunc: func(ctx context.Context) {

			},
			StopFunc: func() {},
		},
		sequenceDispatcher: sequenceDispatcher,
		shipyardRetriever: &shipyardRetrievermock.IShipyardRetrieverMock{
			GetShipyardFunc: func(projectName string) (*keptnv2.Shipyard, error) {
				return common.UnmarshalShipyard(shipyardContent)
			},
			GetCachedShipyardFunc: func(projectName string) (*keptnv2.Shipyard, error) {
				return common.UnmarshalShipyard(shipyardContent)
			},
			GetLatestCommitIDFunc: func(projectName string, stageName string) (string, error) {
				return "latest-commit-id", nil
			},
		},
		sequenceExecutionRepo: sequenceExecutionRepo,
	}
	sc.eventDispatcher.(*fake.IEventDispatcherMock).AddFunc = func(event models.DispatcherEvent, skipQueue bool) error {
		ev := &apimodels.KeptnContextExtendedCE{}
		err := keptnv2.Decode(&event.Event, ev)
		if err != nil {
			return err
		}
		_ = sc.HandleIncomingEvent(*ev, true)
		return nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	sc.run(ctx)
	sc.StartDispatchers(ctx, common.SDModeRW)

	return sc, cancel
}

func filterEvents(eventsCollection []apimodels.KeptnContextExtendedCE, filter common.EventFilter) ([]apimodels.KeptnContextExtendedCE, error) {
	result := []apimodels.KeptnContextExtendedCE{}

	for _, event := range eventsCollection {
		scope, _ := models.NewEventScope(event)
		if filter.Type != "" && *event.Type != filter.Type {
			continue
		}
		if filter.Stage != nil && *filter.Stage != scope.Stage {
			continue
		}

		if filter.Service != nil && *filter.Service != scope.Service {
			continue
		}
		if filter.TriggeredID != nil && *filter.TriggeredID != event.Triggeredid {
			continue
		}
		if filter.KeptnContext != nil && *filter.KeptnContext != event.Shkeptncontext {
			continue
		}
		result = append(result, event)
	}
	return result, nil
}

func getDeploymentFinishedEvent(stage string, service string, triggeredID string, source string, result keptnv2.ResultType) apimodels.KeptnContextExtendedCE {
	return apimodels.KeptnContextExtendedCE{
		Contenttype: "application/json",
		Data: keptnv2.DeploymentFinishedEventData{
			EventData: keptnv2.EventData{
				Project: "test-project",
				Stage:   stage,
				Service: service,
				Status:  keptnv2.StatusSucceeded,
				Result:  result,
				Message: "i am a message",
			},
			Deployment: keptnv2.DeploymentFinishedData{
				DeploymentURIsLocal:  []string{"uri-1", "uri-2"},
				DeploymentURIsPublic: []string{"public-uri-1", "public-uri-2"},
				DeploymentNames:      []string{"deployment-1"},
			},
		},
		Extensions:     nil,
		ID:             "deployment-finished-id",
		Shkeptncontext: "test-context",
		Source:         common.Stringp(source),
		Specversion:    "0.2",
		Time:           time.Now(),
		Triggeredid:    triggeredID,
		Type:           common.Stringp("sh.keptn.event.deployment.finished"),
	}
}

func getErroredDeploymentFinishedEvent(stage string, triggeredID string, source string) apimodels.KeptnContextExtendedCE {
	return apimodels.KeptnContextExtendedCE{
		Contenttype: "application/json",
		Data: keptnv2.DeploymentFinishedEventData{
			EventData: keptnv2.EventData{
				Project: "test-project",
				Stage:   stage,
				Service: "carts",
				Status:  keptnv2.StatusErrored,
				Result:  keptnv2.ResultFailed,
			},
			Deployment: keptnv2.DeploymentFinishedData{
				DeploymentURIsLocal:  []string{"uri-1", "uri-2"},
				DeploymentURIsPublic: []string{"public-uri-1", "public-uri-2"},
				DeploymentNames:      []string{"deployment-1"},
			},
		},
		Extensions:     nil,
		ID:             "deployment-finished-id",
		Shkeptncontext: "test-context",
		Source:         common.Stringp(source),
		Specversion:    "0.2",
		Time:           time.Now(),
		Triggeredid:    triggeredID,
		Type:           common.Stringp("sh.keptn.event.deployment.finished"),
	}
}

func getTestTaskFinishedEvent(stage string, triggeredID string) apimodels.KeptnContextExtendedCE {
	return apimodels.KeptnContextExtendedCE{
		Contenttype: "application/json",
		Data: keptnv2.TestFinishedEventData{
			EventData: keptnv2.EventData{
				Project: "test-project",
				Stage:   stage,
				Service: "carts",
				Status:  keptnv2.StatusSucceeded,
				Result:  keptnv2.ResultPass,
			},
			Test: keptnv2.TestFinishedDetails{
				Start: "start",
				End:   "end",
			},
		},
		Extensions:     nil,
		ID:             "test-finished-id",
		Shkeptncontext: "test-context",
		Source:         common.Stringp("test-source"),
		Specversion:    "0.2",
		Time:           time.Now(),
		Triggeredid:    triggeredID,
		Type:           common.Stringp("sh.keptn.event.test.finished"),
	}
}

func getEvaluationTaskFinishedEvent(stage string, triggeredID string, result keptnv2.ResultType) apimodels.KeptnContextExtendedCE {
	return apimodels.KeptnContextExtendedCE{
		Contenttype: "application/json",
		Data: keptnv2.EvaluationFinishedEventData{
			EventData: keptnv2.EventData{
				Project: "test-project",
				Stage:   stage,
				Service: "carts",
				Status:  keptnv2.StatusSucceeded,
				Result:  result,
			},
			Evaluation: keptnv2.EvaluationDetails{
				Result: string(result),
			},
		},
		Extensions:     nil,
		ID:             "evaluation-finished-id",
		Shkeptncontext: "test-context",
		Source:         common.Stringp("test-source"),
		Specversion:    "0.2",
		Time:           time.Now(),
		Triggeredid:    triggeredID,
		Type:           common.Stringp("sh.keptn.event.evaluation.finished"),
	}
}

func getReleaseTaskFinishedEvent(stage string, triggeredID string) apimodels.KeptnContextExtendedCE {
	return apimodels.KeptnContextExtendedCE{
		Contenttype: "application/json",
		Data: keptnv2.ReleaseFinishedEventData{
			EventData: keptnv2.EventData{
				Project: "test-project",
				Stage:   stage,
				Service: "carts",
				Status:  keptnv2.StatusSucceeded,
				Result:  keptnv2.ResultPass,
			},
		},
		Extensions:     nil,
		ID:             "release-finished-id",
		Shkeptncontext: "test-context",
		Source:         common.Stringp("test-source"),
		Specversion:    "0.2",
		Time:           time.Now(),
		Triggeredid:    triggeredID,
		Type:           common.Stringp("sh.keptn.event.release.finished"),
	}
}

func sendFinishedEvent(sc *ShipyardController, finishedEvent apimodels.KeptnContextExtendedCE) error {
	return sc.HandleIncomingEvent(finishedEvent, true)
}

func sendAndVerifyFinishedEvent(t *testing.T, sc *ShipyardController, finishedEvent apimodels.KeptnContextExtendedCE, eventType, nextEventType string, nextStage string, service string) string {
	err := sc.HandleIncomingEvent(finishedEvent, true)
	require.Nil(t, err)

	scope, _ := models.NewEventScope(finishedEvent)
	if nextStage == "" {
		nextStage = scope.Stage
	}
	// check triggeredEvent collection -> should not contain <eventType>.triggered event anymore
	triggeredEvents, _ := sc.eventRepo.GetEvents("test-project", common.EventFilter{
		Type:    keptnv2.GetTriggeredEventType(eventType),
		Stage:   &scope.Stage,
		Service: common.Stringp(service),
		ID:      &scope.TriggeredID,
		Source:  common.Stringp("shipyard-controller"),
	}, common.TriggeredEvent)
	require.NotContains(t, triggeredEvents, apimodels.KeptnContextExtendedCE{
		ID: scope.TriggeredID,
	})

	// check triggeredEvent collection -> should contain <nextEventType>.triggered event
	triggeredEvents, _ = sc.eventRepo.GetEvents("test-project", common.EventFilter{
		Type:    keptnv2.GetTriggeredEventType(nextEventType),
		Stage:   &nextStage,
		Service: common.Stringp(service),
		Source:  common.Stringp("shipyard-controller"),
	}, common.TriggeredEvent)

	require.NotEmpty(t, triggeredEvents)

	triggeredID := triggeredEvents[0].ID
	ShouldContainEvent(t, triggeredEvents, keptnv2.GetTriggeredEventType(nextEventType), nextStage, nil)

	// check startedEvent collection -> should not contain <eventType>.started event anymore
	startedEvents, _ := sc.eventRepo.GetEvents("test-project", common.EventFilter{
		Type:        keptnv2.GetStartedEventType(eventType),
		Stage:       &scope.Stage,
		Service:     common.Stringp(service),
		TriggeredID: common.Stringp(finishedEvent.Triggeredid),
	}, common.StartedEvent)
	ShouldNotContainEvent(t, startedEvents, keptnv2.GetStartedEventType(eventType), scope.Stage)

	return triggeredID
}

func sendFinishedEventAndVerifyTaskSequenceCompletion(t *testing.T, sc *ShipyardController, finishedEvent apimodels.KeptnContextExtendedCE, eventType, nextStage string) {
	err := sc.HandleIncomingEvent(finishedEvent, true)
	require.Nil(t, err)

	scope, _ := models.NewEventScope(finishedEvent)
	if nextStage == "" {
		nextStage = scope.Stage
	}
	// check triggeredEvent collection -> should not contain <eventType>.triggered event anymore
	triggeredEvents, _ := sc.eventRepo.GetEvents("test-project", common.EventFilter{
		Type:    keptnv2.GetTriggeredEventType(eventType),
		Stage:   &scope.Stage,
		Service: common.Stringp("carts"),
		Source:  common.Stringp("shipyard-controller"),
	}, common.TriggeredEvent)
	ShouldNotContainEvent(t, triggeredEvents, keptnv2.GetTriggeredEventType(eventType), scope.Stage)

	// check startedEvent collection -> should not contain <eventType>.started event anymore
	startedEvents, _ := sc.eventRepo.GetEvents("test-project", common.EventFilter{
		Type:        keptnv2.GetStartedEventType(eventType),
		Stage:       &scope.Stage,
		Service:     common.Stringp("carts"),
		TriggeredID: common.Stringp(finishedEvent.Triggeredid),
	}, common.StartedEvent)
	ShouldNotContainEvent(t, startedEvents, keptnv2.GetStartedEventType(eventType), scope.Stage)
}

func sendAndVerifyPartialFinishedEvent(t *testing.T, sc *ShipyardController, finishedEvent apimodels.KeptnContextExtendedCE, eventType, nextEventType string, nextStage string) {
	err := sc.HandleIncomingEvent(finishedEvent, true)
	require.Nil(t, err)

	scope, _ := models.NewEventScope(finishedEvent)
	if nextStage == "" {
		nextStage = scope.Stage
	}
	// check triggeredEvent collection -> should still contain <eventType>.triggered event
	triggeredEvents, _ := sc.eventRepo.GetEvents("test-project", common.EventFilter{
		Type:    keptnv2.GetTriggeredEventType(eventType),
		Stage:   &scope.Stage,
		Service: common.Stringp("carts"),
		Source:  common.Stringp("shipyard-controller"),
	}, common.TriggeredEvent)
	ShouldContainEvent(t, triggeredEvents, keptnv2.GetTriggeredEventType(eventType), scope.Stage, nil)

	// check triggeredEvent collection -> should not contain <nextEventType>.triggered event
	triggeredEvents, _ = sc.eventRepo.GetEvents("test-project", common.EventFilter{
		Type:    keptnv2.GetTriggeredEventType(nextEventType),
		Stage:   &nextStage,
		Service: common.Stringp("carts"),
		Source:  common.Stringp("shipyard-controller"),
	}, common.TriggeredEvent)

	ShouldNotContainEvent(t, triggeredEvents, keptnv2.GetTriggeredEventType(nextEventType), nextStage)
}

func sendAndVerifyStartedEvent(t *testing.T, sc *ShipyardController, taskName string, triggeredID string, stage string, service string, fromSource string) {
	err := sc.HandleIncomingEvent(getStartedEvent(stage, service, triggeredID, taskName, fromSource), true)
	require.Nil(t, err)
}

func getArtifactDeliveryTriggeredEvent(stage string, commitID string) apimodels.KeptnContextExtendedCE {
	return apimodels.KeptnContextExtendedCE{
		Contenttype: "application/json",
		Data: keptnv2.DeploymentTriggeredEventData{
			EventData: keptnv2.EventData{
				Project: "test-project",
				Stage:   stage,
				Service: "carts",
			},
			ConfigurationChange: struct {
				Values map[string]interface{} `json:"values"`
			}{
				Values: map[string]interface{}{
					"image": "carts",
				},
			},
			Deployment: keptnv2.DeploymentTriggeredData{
				DeploymentURIsPublic: []string{"uri"},
				DeploymentStrategy:   "direct",
			},
		},
		Extensions:     nil,
		ID:             "artifact-delivery-triggered-id",
		Shkeptncontext: "test-context",
		Source:         common.Stringp("test-source"),
		Specversion:    "0.2",
		Time:           time.Now(),
		Triggeredid:    "",
		GitCommitID:    commitID,
		Type:           common.Stringp("sh.keptn.event.dev.artifact-delivery.triggered"),
	}
}

func getArtifactDeliveryTriggeredEventCarts2(stage string, commitID string) apimodels.KeptnContextExtendedCE {
	return apimodels.KeptnContextExtendedCE{
		Contenttype: "application/json",
		Data: keptnv2.DeploymentTriggeredEventData{
			EventData: keptnv2.EventData{
				Project: "test-project",
				Stage:   stage,
				Service: "carts",
			},
			ConfigurationChange: struct {
				Values map[string]interface{} `json:"values"`
			}{
				Values: map[string]interface{}{
					"image": "carts",
				},
			},
			Deployment: keptnv2.DeploymentTriggeredData{
				DeploymentURIsPublic: []string{"uri"},
				DeploymentStrategy:   "direct",
			},
		},
		Extensions:     nil,
		ID:             "artifact-delivery-triggered-id3",
		Shkeptncontext: "test-context",
		Source:         common.Stringp("test-source"),
		Specversion:    "0.2",
		Time:           time.Now(),
		Triggeredid:    "",
		GitCommitID:    commitID,
		Type:           common.Stringp("sh.keptn.event.dev.artifact-delivery.triggered"),
	}
}

func getArtifactDeliveryCarts2TriggeredEvent(stage string, commitID string) apimodels.KeptnContextExtendedCE {
	return apimodels.KeptnContextExtendedCE{
		Contenttype: "application/json",
		Data: keptnv2.DeploymentTriggeredEventData{
			EventData: keptnv2.EventData{
				Project: "test-project",
				Stage:   stage,
				Service: "carts2",
			},
			ConfigurationChange: struct {
				Values map[string]interface{} `json:"values"`
			}{
				Values: map[string]interface{}{
					"image": "carts2",
				},
			},
			Deployment: keptnv2.DeploymentTriggeredData{
				DeploymentURIsPublic: []string{"uri"},
				DeploymentStrategy:   "direct",
			},
		},
		Extensions:     nil,
		ID:             "artifact-delivery-triggered-id2",
		Shkeptncontext: "test-context",
		Source:         common.Stringp("test-source"),
		Specversion:    "0.2",
		Time:           time.Now(),
		Triggeredid:    "",
		GitCommitID:    commitID,
		Type:           common.Stringp("sh.keptn.event.dev.artifact-delivery.triggered"),
	}
}

func getStartedEvent(stage string, service string, triggeredID string, eventType string, source string) apimodels.KeptnContextExtendedCE {
	return apimodels.KeptnContextExtendedCE{
		Contenttype:    "application/json",
		Data:           EventScope{Project: "test-project", Stage: stage, Service: service},
		Extensions:     nil,
		ID:             eventType + "-" + source + "-started-id",
		Shkeptncontext: "test-context",
		Source:         common.Stringp(source),
		Specversion:    "0.2",
		Time:           time.Now(),
		Triggeredid:    triggeredID,
		Type:           common.Stringp(keptnv2.GetStartedEventType(eventType)),
	}
}
