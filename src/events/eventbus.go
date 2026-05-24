package events

import (
	"flux/src/secrets"
	"flux/src/tools"
)

type ResponseEventType int

const (
	Tool ResponseEventType = iota
	Error
	Message
	StreamChunk
)

type ResponseEvent struct {
	Question     string
	Options      []string
	EventType    ResponseEventType
	Message      string
	SubAgent     bool
	SubAgentName string
	SkillName    string
}

type FileChangeType int

const (
	FileChange_Create FileChangeType = iota
	FileChange_Append
	FileChange_Patch
)

type FileChangeEvent struct {
	FileName   string
	ChangeType FileChangeType
	Content    string
	Patches    []tools.ParsedDiff
}

type PlanStepStatus int

const (
	PlanStepPending PlanStepStatus = iota
	PlanStepRunning
	PlanStepCompleted
	PlanStepFailed
)

type PlanStep struct {
	Outline string
	Prompt  string
	Output  string
	Status  PlanStepStatus
}

type PlanStatusEvent struct {
	Title   string
	Steps   []PlanStep
	Current int
	Active  bool
}
type CompactionEvent struct {
	InputTokensBefore  int
	OutputTokensBefore int
}

type EventsManager struct {
	agentOutput   chan ResponseEvent
	agentInput    chan string
	fileChange    chan FileChangeEvent
	subagentInput chan string
	notification  chan Notification
	planStatus    chan PlanStatusEvent
	streamChunk   chan string
	compaction    chan CompactionEvent
	err           chan string
}

type ChannelType int

const (
	AGENT_OUTPUT_CHANNEL ChannelType = iota
	AGENT_INPUT_CHANNEL
	FILE_DIFF_CHANNEL
	SUBAGENT_CHANNEL
	NOTIFICATION_CHANNEL
	AGENT_ERROR_CHANNEL
	PLAN_STATUS_CHANNEL
	STREAM_CHUNK_CHANNEL
	COMPACTION_CHANNEL
)

type NotificationType int

const (
	INFO NotificationType = iota
	ERROR
	DEBUG
)

type Notification struct {
	Type    NotificationType
	Message string
}

func (e *EventsManager) WriteToChannel(channelType ChannelType, data any) {
	switch channelType {
	case AGENT_OUTPUT_CHANNEL:
		event := data.(ResponseEvent)
		event.Message = secrets.RedactForDisplay(event.Message)
		e.agentOutput <- event
		return

	case AGENT_INPUT_CHANNEL:
		e.agentInput <- data.(string)
		return

	case FILE_DIFF_CHANNEL:
		e.fileChange <- data.(FileChangeEvent)
		return

	case SUBAGENT_CHANNEL:
		e.subagentInput <- data.(string)
		return

	case NOTIFICATION_CHANNEL:
		e.notification <- data.(Notification)
		return

	case PLAN_STATUS_CHANNEL:
		e.planStatus <- data.(PlanStatusEvent)
		return

	case STREAM_CHUNK_CHANNEL:
		e.streamChunk <- data.(string)
		return

	case COMPACTION_CHANNEL:
		e.compaction <- data.(CompactionEvent)
	}
}

func (e *EventsManager) ReadFromChannel(channelType ChannelType) any {
	switch channelType {
	case AGENT_OUTPUT_CHANNEL:
		return <-e.agentOutput

	case AGENT_INPUT_CHANNEL:
		return <-e.agentInput

	case FILE_DIFF_CHANNEL:
		return <-e.fileChange

	case SUBAGENT_CHANNEL:
		return <-e.subagentInput

	case NOTIFICATION_CHANNEL:
		return <-e.notification

	case PLAN_STATUS_CHANNEL:
		return <-e.planStatus

	case STREAM_CHUNK_CHANNEL:
		return <-e.streamChunk

	case COMPACTION_CHANNEL:
		return <-e.compaction
	}

	return nil
}

var EventManager = CreateEventManager()

func CreateEventManager() EventsManager {
	return EventsManager{
		agentOutput:   make(chan ResponseEvent),
		agentInput:    make(chan string),
		fileChange:    make(chan FileChangeEvent),
		subagentInput: make(chan string),
		notification:  make(chan Notification),
		planStatus:    make(chan PlanStatusEvent, 8),
		streamChunk:   make(chan string),
		compaction:    make(chan CompactionEvent, 1),
	}
}
