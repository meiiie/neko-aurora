package ledger

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"nekobot.local/native/internal/connections"
	"nekobot.local/native/internal/executors"
	"nekobot.local/native/internal/identity"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

var (
	ErrConflict    = errors.New("nonce_conflict")
	ErrInvalid     = errors.New("invalid_message")
	ErrNotFound    = errors.New("not_found")
	ErrCorrupt     = errors.New("journal_corrupt")
	ErrUnavailable = errors.New("store_unavailable")
	ErrReadOnly    = errors.New("restored_state_read_only")
)

const maxFrame = 1 << 20

type Bot struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	Color           string `json:"color"`
	Revision        int    `json:"revision"`
	Alias           string `json:"alias,omitempty"`
	Description     string `json:"description,omitempty"`
	DisplayRevision uint64 `json:"displayRevision,omitempty"`
}
type Input struct {
	BotID       string `json:"botId"`
	ClientNonce string `json:"clientNonce"`
	Text        string `json:"text"`
	Lane        string `json:"lane"`
}
type Work struct {
	CoreProfileGroupBinding    *CoreProfileGroupBinding    `json:"coreProfileGroupBinding,omitempty"`
	CoreProfileSelection       *CoreProfileSelection       `json:"coreProfileSelection,omitempty"`
	CoreProfilePin             *CoreProfilePin             `json:"coreProfilePin,omitempty"`
	RequesterPin               *identity.VerifiedPrincipal `json:"requesterPin,omitempty"`
	UseBotDefaults             bool                        `json:"useBotDefaults,omitempty"`
	ExpectedBotStateRevision   *uint64                     `json:"expectedBotStateRevision,omitempty"`
	BotState                   *BotState                   `json:"botStatePin,omitempty"`
	ExpectedBotProfileRevision *uint64                     `json:"expectedBotProfileRevision,omitempty"`
	BotProfile                 *BotProfile                 `json:"botProfilePin,omitempty"`
	ContextProtocolVersion     int                         `json:"contextProtocolVersion,omitempty"`
	RuntimeContext             *ContextManifest            `json:"runtimeContext,omitempty"`
	RuntimeContextSHA256       string                      `json:"runtimeContextSHA256,omitempty"`
	ExecutorPolicyVersion      int                         `json:"executorPolicyVersion,omitempty"`
	ExecutorPin                *executors.InstallPin       `json:"executorPin,omitempty"`
	CurrentAttemptLease        *executors.AttemptLease     `json:"currentAttemptLease,omitempty"`
	ToolPolicyVersion          int                         `json:"toolPolicyVersion,omitempty"`
	PluginSelection            *[]PluginSelection          `json:"pluginSelection,omitempty"`
	PluginPins                 []PluginPin                 `json:"pluginPins,omitempty"`
	Collaboration              *CollaborationBinding       `json:"collaboration,omitempty"`
	RequestID                  string                      `json:"requestId"`
	SourceMessageID            string                      `json:"sourceMessageId"`
	OwnerUserID                string                      `json:"ownerUserId"`
	SpaceID                    string                      `json:"spaceId"`
	BotID                      string                      `json:"botId"`
	ConversationID             string                      `json:"conversationId"`
	ClientNonce                string                      `json:"clientNonce"`
	Text                       string                      `json:"text"`
	Lane                       string                      `json:"lane"`
	Status                     string                      `json:"status"`
	DesiredState               string                      `json:"desiredState"`
	Epoch                      uint64                      `json:"epoch"`
	BotConfigRevision          int                         `json:"botConfigRevision"`
	CreatedAt                  string                      `json:"createdAt"`
	CreatedSequence            uint64                      `json:"createdSequence"`
	AttemptID                  string                      `json:"attemptId,omitempty"`
	Reply                      string                      `json:"reply,omitempty"`
	Failure                    string                      `json:"failure,omitempty"`
	Core                       *CoreBinding                `json:"core,omitempty"`
	Artifacts                  []Artifact                  `json:"artifacts,omitempty"`
	Operations                 []Operation                 `json:"operations,omitempty"`
	Context                    *ContextPin                 `json:"contextPin,omitempty"`
	SourceBotID                string                      `json:"sourceBotId,omitempty"`
	RoutedAlias                string                      `json:"routedAlias,omitempty"`
	ContextAccess              *SourceAccess               `json:"contextAccess,omitempty"`
	RuntimeStopped             bool                        `json:"runtimeStopped,omitempty"`
	RuntimeStopReceipt         *StoppedReceipt             `json:"runtimeStopReceipt,omitempty"`
	RuntimeAttemptBinding      *CoreBinding                `json:"runtimeAttemptBinding,omitempty"`
	ContextRevision            uint64                      `json:"contextRevision,omitempty"`
	ContextAttempts            []ContextAttempt            `json:"contextAttempts,omitempty"`
	Account                    *connections.Pin            `json:"accountPin,omitempty"`
	AccountLease               *AccountLease               `json:"accountLease,omitempty"`
	Publication                *Publication                `json:"publication,omitempty"`
	AccountExpiredAt           string                      `json:"accountExpiredAt,omitempty"`
	Resources                  *ResourcePin                `json:"resources,omitempty"`
	Adapter                    *AdapterPin                 `json:"adapterPin,omitempty"`
	TaskRevision               uint64                      `json:"taskRevision,omitempty"`
	TaskLink                   *TaskLink                   `json:"taskLink,omitempty"`
}
type Event struct {
	CoreProfile                    *CoreProfileEvent           `json:"coreProfile,omitempty"`
	ReportDeliverySelectionVersion int                         `json:"reportDeliverySelectionVersion,omitempty"`
	ReportDelivery                 *ReportDelivery             `json:"reportDelivery,omitempty"`
	Challenge                      *SecureChallenge            `json:"challenge,omitempty"`
	ChallengeInput                 *ChallengeInput             `json:"challengeInput,omitempty"`
	ProjectState                   *ProjectState               `json:"projectState,omitempty"`
	ProjectControlInput            *ProjectControlInput        `json:"projectControlInput,omitempty"`
	BotState                       *BotState                   `json:"botState,omitempty"`
	BotControlInput                *BotControlInput            `json:"botControlInput,omitempty"`
	RetentionPreference            *RetentionPreference        `json:"retentionPreference,omitempty"`
	RetentionInput                 *RetentionInput             `json:"retentionInput,omitempty"`
	Conversation                   *Conversation               `json:"conversation,omitempty"`
	ConversationInput              *ConversationCommand        `json:"conversationInput,omitempty"`
	BotProfile                     *BotProfile                 `json:"botProfile,omitempty"`
	CollaborationRecovery          *CollaborationRecoveryEvent `json:"collaborationRecovery,omitempty"`
	ContextClaimVersion            int                         `json:"contextClaimVersion,omitempty"`
	ExecutorClaimVersion           int                         `json:"executorClaimVersion,omitempty"`
	ProjectContinuity              *ProjectContinuity          `json:"projectContinuity,omitempty"`
	Plugin                         *PluginEvent                `json:"plugin,omitempty"`
	Collaboration                  *Collaboration              `json:"collaboration,omitempty"`
	Sequence                       uint64                      `json:"sequence"`
	Type                           string                      `json:"type"`
	Work                           Work                        `json:"work"`
	Bot                            *Bot                        `json:"bot,omitempty"`
	Channel                        *Channel                    `json:"channel,omitempty"`
	Project                        *Project                    `json:"project,omitempty"`
	Upload                         *UploadedFile               `json:"upload,omitempty"`
	ResourceRevision               uint64                      `json:"resourceRevision,omitempty"`
	AdapterSession                 *AdapterSession             `json:"adapterSession,omitempty"`
	Nonce                          string                      `json:"nonce,omitempty"`
	CompletionVersion              int                         `json:"completionVersion,omitempty"`
	ContextRefreshVersion          int                         `json:"contextRefreshVersion,omitempty"`
	TaskControl                    *TaskControlEvent           `json:"taskControl,omitempty"`
}
type Snapshot struct {
	ReportDeliveries     []ReportDelivery      `json:"reportDeliveries"`
	SecureChallenges     []SecureChallenge     `json:"secureChallenges,omitempty"`
	ProjectStates        []ProjectState        `json:"projectStates"`
	BotStates            []BotState            `json:"botStates"`
	ReadOnly             bool                  `json:"readOnly,omitempty"`
	RetentionPreferences []RetentionPreference `json:"retentionPreferences"`
	Conversations        []Conversation        `json:"conversations"`
	BotProfiles          []BotProfile          `json:"botProfiles"`
	Executor             *executors.Snapshot   `json:"executor,omitempty"`
	Plugins              []PluginRecord        `json:"plugins"`
	Collaborations       []Collaboration       `json:"collaborations,omitempty"`
	Sequence             uint64                `json:"sequence"`
	Bots                 []Bot                 `json:"bots"`
	Work                 []Work                `json:"work"`
	Runtime              string                `json:"runtime"`
	Capabilities         []string              `json:"capabilities"`
	Channels             []Channel             `json:"channels,omitempty"`
	Projects             []Project             `json:"projects,omitempty"`
	Files                []UploadedFile        `json:"files,omitempty"`
}
type record struct {
	Version  int    `json:"version"`
	Event    Event  `json:"event"`
	Previous string `json:"previous"`
	Digest   string `json:"digest"`
}
type Store struct {
	documentExtractor        DocumentExtractor
	coreProfiles             CoreProfileService
	coreProfileInspections   map[string]CoreProfileInspectionConsent
	coreProfileConsents      map[string]CoreProfileConsent
	coreProfileMetadata      map[string]CoreProfilesResult
	coreProfileLoads         map[string]chan struct{}
	coreProfileNonces        map[string]Event
	coreProfileRetired       map[string]bool
	coreProfileObservedNow   int64
	identity                 *identity.Authority
	reportDeliveries         map[string]ReportDelivery
	reportDeliveryNonces     map[string]string
	challengeSimulator       bool
	challenges               map[string]SecureChallenge
	challengeNonces          map[string]Event
	challengeProofs          map[string]challengeProof
	projectStates            map[string]ProjectState
	projectStateNonces       map[string]Event
	botStates                map[string]BotState
	botStateNonces           map[string]Event
	readOnly                 bool
	retentionPreferences     map[string]RetentionPreference
	retentionNonces          map[string]Event
	conversations            map[string]Conversation
	conversationNonces       map[string]Event
	syncID                   string
	botProfiles              map[string]BotProfile
	botProfileNonces         map[string]Event
	contextAdmissionRequired bool
	executorRequired         bool
	executor                 *executors.Registry
	plugins                  map[string]PluginRecord
	pluginNonces             map[string]Event
	projectContinuity        map[string]ProjectContinuity
	collaborations           map[string]Collaboration
	collaborationNonces      map[string]string
	collaborationActions     map[string]Event
	mu                       sync.Mutex
	file                     *os.File
	lock                     *os.File
	work                     map[string]Work
	nonces                   map[string]string
	events                   []Event
	digest                   string
	closed                   bool
	poisoned                 bool
	dir                      string
	runtime                  string
	bots                     []Bot
	botNonces                map[string]string
	botCreationInputs        map[string]BotInput
	botNameNonces            map[string]Event
	accounts                 *connections.Store
	now                      func() time.Time
	channels                 map[string]Channel
	channelNonces            map[string]Event
	projects                 map[string]Project
	uploads                  map[string]UploadedFile
	productNonces            map[string]Event
	adapters                 map[string]AdapterSession
	adapterNonces            map[string]Event
	adapterOpenedSequence    uint64
	taskControlNonces        map[string]Event
}

var bots = []Bot{
	{ID: "bot-meiiie", Name: "Meiiie", Color: "#b66a55", Revision: 1, Alias: "meiiie"},
	{ID: "bot-davin", Name: "Davin", Color: "#55765f", Revision: 1, Alias: "davin"},
}

func Open(dir string) (*Store, error) {
	readOnly, err := restoredReadOnly(dir)
	if err != nil {
		return nil, err
	}
	return openStore(dir, readOnly, false)
}
func OpenWithDocumentWorker(dir string, extractor DocumentExtractor) (*Store, error) {
	if extractor == nil || reflect.ValueOf(extractor).Kind() == reflect.Ptr && reflect.ValueOf(extractor).IsNil() {
		return nil, ErrUnavailable
	}
	store, err := Open(dir)
	if err != nil {
		return nil, err
	}
	store.documentExtractor = extractor
	return store, nil
}
func OpenReadOnly(dir string) (*Store, error) { return openStore(dir, true, true) }
func (s *Store) ReadOnly() bool               { return s.readOnly }
func openStore(dir string, readOnly, inspect bool) (*Store, error) {
	if inspect {
		if info, e := os.Lstat(filepath.Join(dir, "writer.lock")); e != nil || !info.Mode().IsRegular() {
			return nil, ErrUnavailable
		}
	} else if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	lock, err := lockWriter(filepath.Join(dir, "writer.lock"))
	if err != nil {
		return nil, fmt.Errorf("writer_lock: %w", err)
	}
	flags := os.O_CREATE | os.O_RDWR
	if readOnly {
		flags = os.O_RDONLY
	}
	file, err := os.OpenFile(filepath.Join(dir, "transactions.jsonl"), flags, 0600)
	if err != nil {
		lock.Close()
		return nil, err
	}
	s := &Store{file: file, lock: lock, work: map[string]Work{}, nonces: map[string]string{}, dir: dir, runtime: "core_unavailable", bots: append([]Bot{}, bots...), botNonces: map[string]string{}, channels: map[string]Channel{}, channelNonces: map[string]Event{}, projects: map[string]Project{}, uploads: map[string]UploadedFile{}, productNonces: map[string]Event{}, adapters: map[string]AdapterSession{}, adapterNonces: map[string]Event{}}
	s.readOnly = readOnly
	s.reportDeliveries = map[string]ReportDelivery{}
	s.reportDeliveryNonces = map[string]string{}
	if readOnly {
		s.runtime = "restored_read_only"
	}
	s.retentionPreferences = map[string]RetentionPreference{}
	s.retentionNonces = map[string]Event{}
	s.conversations = map[string]Conversation{}
	s.conversationNonces = map[string]Event{}
	s.botCreationInputs = map[string]BotInput{}
	s.botNameNonces = map[string]Event{}
	s.botProfiles = map[string]BotProfile{}
	s.botProfileNonces = map[string]Event{}
	s.botStates = map[string]BotState{}
	s.challenges = map[string]SecureChallenge{}
	s.challengeNonces = map[string]Event{}
	s.challengeProofs = map[string]challengeProof{}
	s.projectStates = map[string]ProjectState{}
	s.projectStateNonces = map[string]Event{}
	s.botStateNonces = map[string]Event{}
	s.plugins = map[string]PluginRecord{}
	s.pluginNonces = map[string]Event{}
	s.projectContinuity = map[string]ProjectContinuity{}
	s.taskControlNonces = map[string]Event{}
	s.collaborations = map[string]Collaboration{}
	s.collaborationNonces = map[string]string{}
	s.collaborationActions = map[string]Event{}
	if err = s.replay(); err != nil {
		s.Close()
		return nil, err
	}
	s.adapterOpenedSequence = uint64(len(s.events))
	if !s.readOnly {
		if err = s.recoverAdapterReplyClears(); err != nil {
			s.Close()
			return nil, err
		}
	}
	if err = s.openSyncIdentity(); err != nil {
		s.syncID = ""
	}
	return s, nil
}
func (s *Store) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	s.closed = true
	clear(s.challengeProofs)
	err := s.file.Close()
	lockErr := s.lock.Close()
	if err != nil {
		return err
	}
	return lockErr
}
func digestRecord(r record) (string, error) {
	r.Digest = ""
	b, err := json.Marshal(r)
	if err != nil {
		return "", err
	}
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:]), nil
}
func (s *Store) replay() error {
	reader := bufio.NewReaderSize(s.file, maxFrame+1)
	var offset int64
	for {
		line, err := reader.ReadSlice('\n')
		if len(line) > maxFrame {
			return ErrCorrupt
		}
		if err == io.EOF {
			if len(line) > 0 {
				if s.readOnly {
					return ErrCorrupt
				}
				if err = s.file.Truncate(offset); err != nil {
					return err
				}
				if err = s.file.Sync(); err != nil {
					return err
				}
			}
			_, err = s.file.Seek(0, io.SeekEnd)
			return err
		}
		if err != nil {
			return err
		}
		var r record
		dec := json.NewDecoder(bytes.NewReader(line))
		dec.DisallowUnknownFields()
		if err = dec.Decode(&r); err != nil {
			return ErrCorrupt
		}
		var extra any
		if dec.Decode(&extra) != io.EOF {
			return ErrCorrupt
		}
		d, err := digestRecord(r)
		if err != nil || r.Version != 1 || r.Previous != s.digest || d != r.Digest || r.Event.Sequence != uint64(len(s.events)+1) {
			return ErrCorrupt
		}
		if !validReplay(s, r.Event) {
			return ErrCorrupt
		}
		s.apply(r.Event)
		s.digest = r.Digest
		offset += int64(len(line))
	}
}
func validReplay(s *Store, e Event) bool {
	return validReplayWithArchive(s, e, false)
}
func validReplayWithArchive(s *Store, e Event, linkedControl bool) bool {
	if e.Type == "adapter.reply_clear_checkpoint" {
		return s.validAdapterReplyClearEvent(e)
	}
	if e.CoreProfile != nil || strings.HasPrefix(e.Type, "core_profile.") {
		return s.validCoreProfileEvent(e)
	}
	if e.Nonce != "" && s.coreProfileNonces[e.Nonce].Nonce != "" || e.Type == "work.accepted" && s.coreProfileNonces[e.Work.ClientNonce].Nonce != "" {
		return false
	}
	if e.ReportDelivery != nil || e.ReportDeliverySelectionVersion != 0 || e.Type == "report.delivered" {
		return s.validReportDeliveryEvent(e)
	}
	if s.reportEventNonceUsed(e) {
		return false
	}
	if e.Challenge != nil || e.ChallengeInput != nil || strings.HasPrefix(e.Type, "challenge.") {
		return s.validChallengeEvent(e)
	}
	if s.challengeEventNonceUsed(e) {
		return false
	}
	if e.ProjectState != nil || e.ProjectControlInput != nil || e.Type == "project.state_updated" {
		return s.validProjectStateEvent(e)
	}
	if e.BotState != nil || e.BotControlInput != nil || e.Type == "bot.state_updated" {
		return s.validBotStateEvent(e)
	}
	if e.RetentionPreference != nil || e.RetentionInput != nil || e.Type == "retention.preference_saved" {
		return s.validRetentionEvent(e)
	}
	if e.Conversation != nil || e.ConversationInput != nil || strings.HasPrefix(e.Type, "conversation.") {
		return s.validConversationEvent(e)
	}
	if e.BotProfile != nil || e.Type == "bot.profile_updated" {
		return s.validBotProfileEvent(e)
	}
	if e.CollaborationRecovery != nil || e.Type == "collaboration.recovered" {
		return s.validCollaborationRecoveryEvent(e)
	}
	if e.ContextClaimVersion != 0 && (e.ContextClaimVersion != 1 || e.Type != "work.claimed") {
		return false
	}
	if e.ExecutorClaimVersion != 0 && (e.ExecutorClaimVersion != 1 || e.Type != "work.claimed") {
		return false
	}
	if e.Work.RequestID != "" && !validExecutorWork(e.Work) {
		return false
	}
	if e.Plugin != nil && e.ProjectContinuity != nil {
		return false
	}
	if e.Type == "plugin.lifecycle" {
		return s.validPluginEvent(e)
	}
	if e.Plugin != nil {
		return false
	}
	if e.ProjectContinuity != nil || e.Type == "project.continuity_saved" {
		return s.validContinuityEvent(e)
	}
	if strings.HasPrefix(e.Type, "collaboration.") {
		return s.validCollaborationEvent(e)
	}
	if e.Collaboration != nil {
		return false
	}
	if e.Type == "task.control" {
		return s.validTaskControlEvent(e)
	}
	if e.TaskControl != nil {
		return false
	}
	if e.Type == "work.control_released" {
		return s.validControlRelease(e)
	}
	if e.Type == "adapter.created" {
		return s.validAdapterEvent(e)
	}
	if e.AdapterSession != nil {
		return false
	}
	if e.ContextRefreshVersion != 0 && ((e.ContextRefreshVersion < 1 || e.ContextRefreshVersion > 5) || e.Type != "work.context_refreshed") {
		return false
	}
	if e.CompletionVersion != 0 && (e.CompletionVersion != 1 || e.Type != "work.completed") {
		return false
	}
	if e.Type == "project.created" || e.Type == "project.updated" || e.Type == "file.uploaded" || e.Type == "file.access" {
		return s.validProductEvent(e)
	}
	if e.Project != nil || e.Upload != nil || e.ResourceRevision != 0 {
		return false
	}
	if e.Type == "channel.created" || e.Type == "channel.updated" {
		return s.validChannelEvent(e)
	}
	if e.Channel != nil {
		return false
	}
	if e.Type == "bot.created" {
		return s.validBotEvent(e)
	}
	if e.Type == "bot.renamed" {
		return s.validBotNameEvent(e)
	}
	if e.Bot != nil || e.Nonce != "" {
		return false
	}
	w := e.Work
	if w.RequestID == "" || w.SourceMessageID == "" || w.OwnerUserID != "local-owner" || w.SpaceID != "local-space" {
		return false
	}
	switch e.Type {
	case "work.accepted":
		if !s.validCoreProfileIntake(w) {
			return false
		}
		if !validRequesterWork(e.Work) {
			return false
		}
		for _, delivery := range s.reportDeliveries {
			if delivery.SourceMessageID == w.SourceMessageID {
				return false
			}
		}
		if w.Collaboration == nil && !linkedControl {
			if w.Resources != nil && s.archivedProject(w.Resources.Project.ID) {
				return false
			}
			if s.archivedBot(w.BotID) || w.SourceBotID != "" && s.archivedBot(w.SourceBotID) {
				return false
			}
			if conversation, ok := s.conversation(w.ConversationID); ok && conversation.Archived {
				return false
			}
		}
		if !s.validProfileIntake(w) {
			return false
		}
		if !s.validBotStateIntake(w, linkedControl) {
			return false
		}
		if !s.validNewRuntimeContext(w) {
			return false
		}
		if !s.validPluginWork(w) {
			return false
		}
		if w.TaskRevision != 0 || w.TaskLink != nil {
			return false
		}
		if !s.validAdapterPin(w) {
			return false
		}
		if w.Resources != nil && w.Collaboration == nil {
			pin, err := s.pinResources(w, SubmissionScope{ProjectID: w.Resources.Project.ID, ProjectRevision: w.Resources.Project.Revision, FileIDs: w.Resources.RequestedFileIDs})
			if err != nil || !reflect.DeepEqual(pin, w.Resources) || w.Context == nil || w.Context.Version < 3 {
				return false
			}
		}
		if w.AccountLease != nil || (w.Account != nil && !connections.ValidPin(*w.Account)) {
			return false
		}
		if w.AccountExpiredAt != "" {
			return false
		}
		if w.Context != nil {
			if (w.Context.Version < 1 || w.Context.Version > 5) || !reflect.DeepEqual(w.Context, s.captureContextVersion(w, w.Context.Version)) {
				return false
			}
		}
		if w.Core != nil || len(w.Operations) > 0 || len(w.Artifacts) > 0 || w.Reply != "" || w.AttemptID != "" || w.Failure != "" || w.ContextAccess != nil || w.RuntimeStopped || w.RuntimeStopReceipt != nil || w.RuntimeAttemptBinding != nil || w.ContextRevision != 0 || len(w.ContextAttempts) != 0 {
			return false
		}
		if s.validInput(Input{w.BotID, w.ClientNonce, w.Text, w.Lane}) != nil || !s.validRoute(w) || w.Status != "waiting_host" || w.DesiredState != "running" || w.Epoch != 1 {
			return false
		}
		if _, ok := s.work[w.RequestID]; ok {
			return false
		}
		if _, ok := s.nonces[workNonceKey(w)]; ok {
			return false
		}
		return s.validCollaborationWork(w) && s.validPublication(w) && w.BotConfigRevision == 1 && w.CreatedSequence == e.Sequence
	case "work.cancelled":
		old, ok := s.work[w.RequestID]
		if ok && old.Status != "waiting_host" {
			return validRuntimeReplay(s, e)
		}
		if !ok || old.Status != "waiting_host" || w.Status != "cancelled" || w.DesiredState != "cancelled" || w.Epoch != old.Epoch+1 {
			return false
		}
		old.Status = w.Status
		old.DesiredState = w.DesiredState
		old.Epoch = w.Epoch
		return reflect.DeepEqual(old, w)
	default:
		return validRuntimeReplay(s, e)
	}
}
func (s *Store) apply(e Event) {
	if e.Type == "adapter.reply_clear_checkpoint" {
		s.events = append(s.events, e)
		return
	}
	if e.CoreProfile != nil {
		s.applyCoreProfile(e)
		return
	}
	if e.ReportDelivery != nil {
		d := copyReportDelivery(*e.ReportDelivery)
		e.ReportDelivery = &d
		s.reportDeliveries[d.ID] = d
		s.reportDeliveryNonces[d.ClientNonce] = d.ID
		s.events = append(s.events, e)
		return
	}
	if e.Challenge != nil {
		c, in := *e.Challenge, *e.ChallengeInput
		e.Challenge = &c
		e.ChallengeInput = &in
		s.challenges[c.ID] = c
		s.challengeNonces[e.Nonce] = e
		s.events = append(s.events, e)
		return
	}
	if e.ProjectState != nil {
		p, in := *e.ProjectState, *e.ProjectControlInput
		e.ProjectState = &p
		e.ProjectControlInput = &in
		s.projectStates[p.ProjectID] = p
		s.projectStateNonces[e.Nonce] = e
		s.events = append(s.events, e)
		return
	}
	if e.BotState != nil {
		p, in := *e.BotState, *e.BotControlInput
		e.BotState = &p
		e.BotControlInput = &in
		s.botStates[p.BotID] = p
		s.botStateNonces[in.ClientNonce] = e
		s.events = append(s.events, e)
		return
	}
	if e.RetentionPreference != nil {
		p, in := *e.RetentionPreference, *e.RetentionInput
		e.RetentionPreference = &p
		e.RetentionInput = &in
		s.retentionPreferences[p.ConversationID] = p
		s.retentionNonces[in.ClientNonce] = e
		s.events = append(s.events, e)
		return
	}
	if e.Conversation != nil {
		c, in := *e.Conversation, *e.ConversationInput
		e.Conversation = &c
		e.ConversationInput = &in
		s.conversations[c.ID] = c
		s.conversationNonces[in.ClientNonce] = e
		s.events = append(s.events, e)
		return
	}
	if e.BotProfile != nil {
		p := *e.BotProfile
		e.BotProfile = &p
		s.botProfiles[p.BotID] = p
		s.botProfileNonces[e.Nonce] = e
		s.events = append(s.events, e)
		return
	}
	if e.Plugin != nil {
		e.Plugin = copyPluginEvent(e.Plugin)
		s.plugins[e.Plugin.Plugin.ID] = copyPlugin(e.Plugin.Plugin)
		s.pluginNonces[e.Plugin.Input.ClientNonce] = e
	}
	if e.ProjectContinuity != nil {
		c := cloneContinuity(*e.ProjectContinuity)
		e.ProjectContinuity = &c
		s.projectContinuity[continuityKey(c.ProjectID, c.ChannelID)] = c
		s.productNonces[e.Nonce] = e
		s.events = append(s.events, e)
		return
	}
	if e.Collaboration != nil {
		if e.CollaborationRecovery != nil {
			c := *e.CollaborationRecovery
			e.CollaborationRecovery = &c
			s.work[e.Work.RequestID] = copyWork(e.Work)
			e.Work = copyWork(e.Work)
		}
		g := copyCollaboration(*e.Collaboration)
		e.Collaboration = &g
		s.collaborations[g.ID] = g
		s.collaborationNonces[g.ClientNonce] = g.ID
		if e.Nonce != "" {
			s.collaborationActions[e.Nonce] = e
		}
		s.events = append(s.events, e)
		return
	}
	if e.TaskControl != nil {
		e.TaskControl = copyTaskControl(e.TaskControl)
		s.work[e.Work.RequestID] = copyWork(e.Work)
		if e.TaskControl.Related != nil {
			w := copyWork(*e.TaskControl.Related)
			s.work[w.RequestID] = w
			s.nonces[workNonceKey(w)] = w.RequestID
		}
		s.taskControlNonces[e.TaskControl.Input.ClientNonce] = e
		e.Work = copyWork(e.Work)
		s.events = append(s.events, e)
		return
	}
	if e.AdapterSession != nil {
		a := *e.AdapterSession
		e.AdapterSession = &a
		s.adapters[a.ID] = a
		s.adapterNonces[e.Nonce] = e
		s.events = append(s.events, e)
		return
	}
	if e.Project != nil || e.Upload != nil {
		if e.Project != nil {
			p := cloneProject(*e.Project)
			e.Project = &p
			s.projects[p.ID] = p
		}
		if e.Upload != nil {
			f := *e.Upload
			e.Upload = &f
			s.uploads[f.ID] = f
		}
		s.productNonces[e.Nonce] = e
		s.events = append(s.events, e)
		return
	}
	if e.Channel != nil {
		c := cloneChannel(*e.Channel)
		e.Channel = &c
		s.channels[c.ID] = c
		s.channelNonces[e.Nonce] = e
		s.events = append(s.events, e)
		return
	}
	if e.Bot != nil {
		bot := *e.Bot
		e.Bot = &bot
		if e.Type == "bot.renamed" {
			for i := range s.bots {
				if s.bots[i].ID == bot.ID {
					s.bots[i] = bot
					break
				}
			}
			s.botNameNonces[e.Nonce] = e
		} else {
			s.bots = append(s.bots, bot)
			s.botNonces[e.Nonce] = bot.ID
			s.botCreationInputs[e.Nonce] = BotInput{Name: bot.Name, Alias: bot.Alias, Description: bot.Description, ClientNonce: e.Nonce}
		}
		s.events = append(s.events, e)
		return
	}
	s.work[e.Work.RequestID] = copyWork(e.Work)
	s.nonces[workNonceKey(e.Work)] = e.Work.RequestID
	e.Work = copyWork(e.Work)
	s.events = append(s.events, e)
}
func (s *Store) commit(e Event) error {
	if s.readOnly {
		return ErrReadOnly
	}
	if s.closed || s.poisoned {
		return ErrUnavailable
	}
	adapterClear := e.Type == "adapter.reply_clear_checkpoint"
	if adapterClear {
		candidate := e
		candidate.Sequence = uint64(len(s.events) + 1)
		if !s.validAdapterReplyClearEvent(candidate) {
			return ErrInvalid
		}
	}
	if !adapterClear && e.CoreProfile == nil && (e.Nonce != "" && s.coreProfileNonces[e.Nonce].Nonce != "" || e.Type == "work.accepted" && s.coreProfileNonces[e.Work.ClientNonce].Nonce != "") {
		return ErrConflict
	}
	if !adapterClear && e.ReportDelivery == nil && s.reportEventNonceUsed(e) {
		return ErrConflict
	}
	if !adapterClear && e.Challenge == nil {
		if s.challengeEventNonceUsed(e) {
			return ErrConflict
		}
	}
	e.Sequence = uint64(len(s.events) + 1)
	r := record{Version: 1, Event: e, Previous: s.digest}
	d, err := digestRecord(r)
	if err != nil {
		return err
	}
	r.Digest = d
	line, err := json.Marshal(r)
	if err != nil {
		return err
	}
	if s.privateSimulationValue(line) {
		return ErrInvalid
	}
	line = append(line, '\n')
	if len(line) > maxFrame {
		return ErrInvalid
	}
	n, err := s.file.Write(line)
	if err != nil || n != len(line) {
		s.poisoned = true
		return ErrUnavailable
	}
	if err = s.file.Sync(); err != nil {
		s.poisoned = true
		return ErrUnavailable
	}
	s.apply(e)
	s.digest = d
	return nil
}
func nonceKey(bot, nonce string) string { return bot + "\x00" + nonce }
func (s *Store) validInput(in Input) error {
	found := false
	for _, bot := range s.bots {
		if bot.ID == in.BotID {
			found = true
		}
	}
	if !found || !utf8.ValidString(in.Text) || strings.TrimSpace(in.Text) == "" || len(in.Text) > 32768 || len(in.ClientNonce) < 8 || len(in.ClientNonce) > 128 {
		return ErrInvalid
	}
	for _, c := range in.ClientNonce {
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '-' || c == '_') {
			return ErrInvalid
		}
	}
	if in.Lane != "foreground" && in.Lane != "background" {
		return ErrInvalid
	}
	return nil
}
func newID(prefix string) (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return prefix + hex.EncodeToString(b[:]), nil
}
func (s *Store) Accept(in Input) (Work, bool, error) {
	if in.Lane == "" {
		in.Lane = "foreground"
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.acceptLocked(in, nil, "")
}
func (s *Store) acceptLocked(in Input, account *connections.Pin, channelID string, options ...SubmissionScope) (Work, bool, error) {
	scope := SubmissionScope{ChannelID: channelID}
	if len(options) > 0 {
		scope = options[0]
	}
	if err := s.validInput(in); err != nil {
		return Work{}, false, err
	}
	if s.closed || s.poisoned {
		return Work{}, false, ErrUnavailable
	}
	if scope.ExpectedBotProfileRevision != nil && *scope.ExpectedBotProfileRevision > 1<<53-1 {
		return Work{}, false, ErrInvalid
	}
	if scope.ExpectedBotStateRevision != nil && *scope.ExpectedBotStateRevision >= 1<<53 {
		return Work{}, false, ErrInvalid
	}
	if s.identity != nil && scope.requester == nil {
		return Work{}, false, ErrForbidden
	}
	if scope.requester != nil && (s.identity == nil || !identity.ValidPrincipal(*scope.requester)) {
		return Work{}, false, ErrForbidden
	}
	if id, ok := s.findInputNonce(in, channelID, scope.requester); ok {
		old := s.work[id]
		if !reflect.DeepEqual(old.CoreProfileSelection, scope.CoreProfileSelection) {
			return Work{}, false, ErrConflict
		}
		if scope.ExpectedInstallationPin != nil && (old.ExecutorPin == nil || *old.ExecutorPin != *scope.ExpectedInstallationPin) {
			return Work{}, false, ErrConflict
		}
		if originalBot(old) != in.BotID || old.Text != in.Text || old.Lane != in.Lane || (channelID != "" && old.ConversationID != channelID) {
			return Work{}, false, ErrConflict
		}
		if !reflect.DeepEqual(old.PluginSelection, scope.PluginSelection) || !reflect.DeepEqual(old.Account, account) || !reflect.DeepEqual(old.Adapter, scope.Adapter) {
			return Work{}, false, ErrConflict
		}
		if !reflect.DeepEqual(old.ExpectedBotProfileRevision, scope.ExpectedBotProfileRevision) {
			return Work{}, false, ErrConflict
		}
		if old.UseBotDefaults != scope.UseBotDefaults || !reflect.DeepEqual(old.ExpectedBotStateRevision, scope.ExpectedBotStateRevision) {
			return Work{}, false, ErrConflict
		}
		if !resourceSelectionEqual(old.Resources, scope) || (scope.ExpectedChannelRevision != nil && (old.Publication == nil || old.Publication.AudienceRevision != *scope.ExpectedChannelRevision)) {
			return Work{}, false, ErrConflict
		}
		return copyWork(old), true, nil
	}
	intakeConversation := channelID
	if intakeConversation == "" {
		intakeConversation = "conversation-" + in.BotID
	}
	if c, ok := s.conversation(intakeConversation); ok && c.Archived {
		return Work{}, false, ErrConflict
	}
	req, err := newID("req-")
	if err != nil {
		return Work{}, false, err
	}
	msg, err := newID("msg-")
	if err != nil {
		return Work{}, false, err
	}
	w := Work{RequestID: req, SourceMessageID: msg, OwnerUserID: "local-owner", SpaceID: "local-space", BotID: in.BotID, ConversationID: "conversation-" + in.BotID, ClientNonce: in.ClientNonce, Text: in.Text, Lane: in.Lane, Status: "waiting_host", DesiredState: "running", Epoch: 1, BotConfigRevision: 1, CreatedAt: time.Now().UTC().Format(time.RFC3339Nano), CreatedSequence: uint64(len(s.events) + 1)}
	w.RequesterPin = copyRequester(scope.requester)
	if scope.CoreProfileSelection != nil {
		w.CreatedAt = s.clock().UTC().Format(time.RFC3339Nano)
		selection := *scope.CoreProfileSelection
		w.CoreProfileSelection = &selection
	}
	if target, alias, err := s.resolveRoute(in.Text); err != nil {
		return Work{}, false, err
	} else if target != "" && target != in.BotID {
		w.SourceBotID = in.BotID
		w.BotID = target
		w.RoutedAlias = alias

	}
	if s.archivedBot(in.BotID) || s.archivedBot(w.BotID) {
		return Work{}, false, ErrConflict
	}
	currentState, found := s.botState(w.BotID)
	if !found {
		return Work{}, false, ErrNotFound
	}
	if scope.ExpectedBotStateRevision != nil {
		if currentState.Revision != *scope.ExpectedBotStateRevision {
			return Work{}, false, ErrConflict
		}
		v := *scope.ExpectedBotStateRevision
		w.ExpectedBotStateRevision = &v
	}
	if scope.UseBotDefaults {
		if scope.ExpectedBotStateRevision == nil {
			return Work{}, false, ErrInvalid
		}
		w.UseBotDefaults = true
		w.BotState = copyBotState(&currentState)
	}
	if channelID != "" {
		w.ConversationID = channelID
	}
	channel, ok := s.channel(w.ConversationID)
	if !ok {
		return Work{}, false, ErrNotFound
	}
	if scope.ExpectedChannelRevision != nil && channel.Revision != *scope.ExpectedChannelRevision {
		return Work{}, false, ErrConflict
	}
	w.Publication = &Publication{Version: 1, ActorID: w.OwnerUserID, Platform: "nekobot-local", SourceChannelID: w.ConversationID, SourceMessageID: w.SourceMessageID, ReplyChannelID: w.ConversationID, ReplyThreadID: "thread-" + w.SourceMessageID, AudienceRevision: channel.Revision}
	w.Publication.ActorID = requesterActor(w)
	if scope.Adapter != nil {
		p := *scope.Adapter
		w.Adapter = &p
	}
	if !s.validAdapterPin(w) {
		return Work{}, false, ErrConflict
	}
	if !s.publicationAllowed(w) {
		return Work{}, false, ErrConflict
	}
	w.Resources, err = s.pinResources(w, scope)
	if err != nil {
		return Work{}, false, err
	}
	w.ToolPolicyVersion = 1
	s.pinBotProfile(&w)
	if validChannelID(w.ConversationID) {
		w.ToolPolicyVersion = 2
		w.Context = &ContextPin{Version: 5, PublishedReports: copyPublishedReports(s.capturePublishedReports(w, uint64(len(s.events))).Pins)}
	}
	w.PluginSelection = copySelection(scope.PluginSelection)
	w.PluginPins, err = s.selectPlugins(w, w.PluginSelection)
	if err != nil {
		return Work{}, false, err
	}
	s.pinBotProfile(&w)
	if scope.ExpectedBotProfileRevision != nil {
		p, _ := s.profile(w.BotID)
		if p.Revision != *scope.ExpectedBotProfileRevision {
			return Work{}, false, ErrConflict
		}
		revision := *scope.ExpectedBotProfileRevision
		w.ExpectedBotProfileRevision = &revision
	}
	w.Context = s.captureContext(w)
	w.Account = account
	if err = s.attachRuntimeContext(&w); err != nil {
		return Work{}, false, err
	}
	if err = s.pinNewExecutor(&w); err != nil {
		return Work{}, false, err
	}
	if scope.CoreProfileSelection != nil && !scope.coreProfileVerified {
		return Work{}, false, ErrForbidden
	}
	if err = s.pinCoreProfile(&w, scope.CoreProfileSelection); err != nil {
		return Work{}, false, err
	}
	if !boundedRuntimePrompt(w) {
		return Work{}, false, ErrInvalid
	}
	commit := func() error { return s.commit(Event{Type: "work.accepted", Work: w}) }
	if scope.ExpectedInstallationPin != nil {
		if w.ExecutorPin == nil || *w.ExecutorPin != *scope.ExpectedInstallationPin {
			return Work{}, false, ErrConflict
		}
		err = s.withInstallPin(w.ExecutorPolicyVersion, w.ExecutorPin, commit)
	} else {
		err = commit()
	}
	if err != nil {
		return Work{}, false, err
	}
	return copyWork(w), false, nil
}
func (s *Store) Cancel(id string) (Work, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cancelLocked(id)
}
func (s *Store) cancelLocked(id string) (Work, error) {
	w, ok := s.work[id]
	if !ok {
		return Work{}, ErrNotFound
	}
	if s.closed || s.poisoned {
		return Work{}, ErrUnavailable
	}
	if w.Status == "cancelled" {
		return copyWork(w), nil
	}
	if w.Status != "waiting_host" && w.Status != "starting" && w.Status != "running" && w.Status != "recovery_required" && w.Status != "suspending" && w.Status != "suspended" && w.Status != "paused" && w.Status != "waiting_control_stop" {
		return Work{}, ErrConflict
	}
	active := w.Status == "starting" || w.Status == "running" || (w.Status == "suspending" && !w.RuntimeStopped)
	w.Status = "cancelled"
	if active {
		w.Status = "cancelling"
	}
	w.DesiredState = "cancelled"
	w.Epoch++
	typ := "work.cancelled"
	if active {
		typ = "work.cancelling"
	}
	if err := s.commit(Event{Type: typ, Work: w}); err != nil {
		return Work{}, err
	}
	return copyWork(w), nil
}
func (s *Store) Snapshot() Snapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.snapshotLocked()
}
func (s *Store) snapshotLocked() Snapshot {
	out := Snapshot{Sequence: uint64(len(s.events)), Bots: append([]Bot{}, s.bots...), Work: []Work{}, Runtime: s.runtime, Capabilities: []string{"context_access_v1", "context_refresh_v1"}}
	out.ReportDeliveries = []ReportDelivery{}
	out.Capabilities = append(out.Capabilities, "local_report_delivery_v1")
	out.Capabilities = append(out.Capabilities, "local_core_discovery_v1")
	if s.coreProfilesEnabled() {
		out.Capabilities = append(out.Capabilities, "local_core_profiles_v1")
		if s.executorRequired && s.executor != nil && s.contextAdmissionRequired {
			out.Capabilities = append(out.Capabilities, "local_core_profile_collaboration_v1")
		}
	}
	out.Capabilities = append(out.Capabilities, "local_published_report_context_v1")
	for _, d := range s.reportDeliveries {
		out.ReportDeliveries = append(out.ReportDeliveries, s.publicReportDelivery(d))
	}
	sort.Slice(out.ReportDeliveries, func(i, j int) bool {
		return out.ReportDeliveries[i].CreatedSequence < out.ReportDeliveries[j].CreatedSequence
	})
	out.ReadOnly = s.readOnly
	if s.challengeSimulator && !s.readOnly {
		out.Capabilities = append(out.Capabilities, "local_secure_challenge_simulator_v1")
	}
	for _, c := range s.challenges {
		out.SecureChallenges = append(out.SecureChallenges, c)
	}
	sort.Slice(out.SecureChallenges, func(i, j int) bool { return out.SecureChallenges[i].ID < out.SecureChallenges[j].ID })
	out.BotStates = s.botStatesLocked()
	out.ProjectStates = []ProjectState{}
	for id := range s.projects {
		state, _ := s.projectState(id)
		out.ProjectStates = append(out.ProjectStates, state)
	}
	sort.Slice(out.ProjectStates, func(i, j int) bool { return out.ProjectStates[i].ProjectID < out.ProjectStates[j].ProjectID })
	out.Capabilities = append(out.Capabilities, "local_project_states_v1")
	out.Capabilities = append(out.Capabilities, "local_source_explanation_v1")
	out.Capabilities = append(out.Capabilities, "local_public_history_export_v1")
	out.Capabilities = append(out.Capabilities, "local_metadata_receipts_v1")
	out.Capabilities = append(out.Capabilities, "local_bot_states_v1", "local_bot_default_intake_cas_v1")
	if s.readOnly {
		out.Runtime = "restored_read_only"
		out.Capabilities = append(out.Capabilities, "restored_state_readonly_v1")
	}
	out.Conversations = s.conversationsLocked()
	out.RetentionPreferences = s.retentionPreferencesLocked()
	out.Capabilities = append(out.Capabilities, "local_conversations_v1", "local_history_export_v1", "local_retention_preview_v1")
	for _, bot := range s.bots {
		p, _ := s.profile(bot.ID)
		out.BotProfiles = append(out.BotProfiles, p)
	}
	if s.executorRequired && s.executor != nil {
		snapshot := s.executor.Snapshot()
		out.Executor = &snapshot
		out.Capabilities = append(out.Capabilities, "local_executor_lease_v1")
		if s.contextAdmissionRequired {
			out.Capabilities = append(out.Capabilities, "local_service_crash_recovery_v1", "local_collaboration_service_recovery_v1")
		}
	}
	out.Capabilities = append(out.Capabilities, "local_collaboration_v1")
	for _, g := range s.collaborations {
		out.Collaborations = append(out.Collaborations, s.snapshotCollaborationLocked(g))
	}
	sort.Slice(out.Collaborations, func(i, j int) bool { return out.Collaborations[i].ID < out.Collaborations[j].ID })
	for _, c := range s.channels {
		out.Channels = append(out.Channels, cloneChannel(c))
	}
	sort.Slice(out.Channels, func(i, j int) bool { return out.Channels[i].ID < out.Channels[j].ID })
	out.Capabilities = append(out.Capabilities, "local_channels_v1", "account_lease_fixture_v1")
	out.Capabilities = append(out.Capabilities, "local_bot_display_names_v1")
	out.Capabilities = append(out.Capabilities, "local_bot_profiles_v1")
	out.Capabilities = append(out.Capabilities, "local_bot_profile_intake_cas_v1")
	out.Capabilities = append(out.Capabilities, "local_docx_text_v1", "local_file_text_preview_v1")
	out.Capabilities = append(out.Capabilities, "local_office_text_v1")
	if s.syncID != "" {
		out.Capabilities = append(out.Capabilities, "local_sync_simulation_v1")
	}
	out.Capabilities = append(out.Capabilities, "local_channel_intake_cas_v1", "shared_channel_context_v3", "local_projects_v1", "local_uploads_v1", "local_upload_receipts_v1")
	out.Capabilities = append(out.Capabilities, "local_surface_bridge_v1")
	out.Capabilities = append(out.Capabilities, "local_adapter_installation_intake_cas_v1")
	out.Capabilities = append(out.Capabilities, "local_task_controls_v1", "queued_task_pause_v1", "local_plugin_lifecycle_v1", "trusted_local_tool_broker_v1")
	out.Plugins = s.pluginsLocked()
	out.Capabilities = append(out.Capabilities, "local_command_receipts_v1")
	out.Capabilities = append(out.Capabilities, "local_project_continuity_v1", "local_continuity_intake_cas_v1")
	for _, p := range s.projects {
		out.Projects = append(out.Projects, cloneProject(p))
	}
	sort.Slice(out.Projects, func(i, j int) bool { return out.Projects[i].ID < out.Projects[j].ID })
	for _, f := range s.uploads {
		out.Files = append(out.Files, publicFile(f))
	}
	sort.Slice(out.Files, func(i, j int) bool { return out.Files[i].ID < out.Files[j].ID })
	for _, w := range s.work {
		out.Work = append(out.Work, s.redactPublishedWork(w))
	}
	sort.Slice(out.Work, func(i, j int) bool { return out.Work[i].CreatedSequence < out.Work[j].CreatedSequence })
	return out
}
func (s *Store) Events(after uint64) ([]Event, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.eventsLocked(after)
}
func (s *Store) eventsLocked(after uint64) ([]Event, error) {
	if after > uint64(len(s.events)) {
		return nil, ErrConflict
	}
	out := append([]Event{}, s.events[int(after):]...)
	for i := range out {
		if out[i].ReportDelivery != nil {
			d := s.publicReportDelivery(*out[i].ReportDelivery)
			out[i].ReportDelivery = &d
		}
		if out[i].ProjectState != nil {
			p := *out[i].ProjectState
			out[i].ProjectState = &p
		}
		if out[i].ProjectControlInput != nil {
			in := *out[i].ProjectControlInput
			out[i].ProjectControlInput = &in
		}
		if out[i].Challenge != nil {
			c := *out[i].Challenge
			out[i].Challenge = &c
		}
		if out[i].ChallengeInput != nil {
			in := *out[i].ChallengeInput
			out[i].ChallengeInput = &in
		}
		out[i].BotState = copyBotState(out[i].BotState)
		if out[i].BotControlInput != nil {
			c := *out[i].BotControlInput
			out[i].BotControlInput = &c
		}
		if out[i].RetentionPreference != nil {
			p, in := *out[i].RetentionPreference, *out[i].RetentionInput
			out[i].RetentionPreference = &p
			out[i].RetentionInput = &in
		}
		if out[i].Conversation != nil {
			c, in := *out[i].Conversation, *out[i].ConversationInput
			out[i].Conversation = &c
			out[i].ConversationInput = &in
		}
		out[i].BotProfile = copyBotProfile(out[i].BotProfile)
		out[i].Plugin = copyPluginEvent(out[i].Plugin)
		out[i].Work = s.publicWorkLocked(out[i].Work)
		if out[i].ProjectContinuity != nil {
			c := publicContinuity(*out[i].ProjectContinuity)
			out[i].ProjectContinuity = &c
		}
		if out[i].Collaboration != nil {
			g := PublicContinuityCollaboration(*out[i].Collaboration)
			out[i].Collaboration = &g
		}
		if out[i].CollaborationRecovery != nil {
			c := *out[i].CollaborationRecovery
			out[i].CollaborationRecovery = &c
		}
		out[i].TaskControl = copyTaskControl(out[i].TaskControl)
		if out[i].TaskControl != nil && out[i].TaskControl.Related != nil {
			w := s.publicWorkLocked(*out[i].TaskControl.Related)
			out[i].TaskControl.Related = &w
		}
		if out[i].AdapterSession != nil {
			a := *out[i].AdapterSession
			out[i].AdapterSession = &a
		}
		if out[i].Bot != nil {
			bot := *out[i].Bot
			out[i].Bot = &bot
		}
		if out[i].Channel != nil {
			c := cloneChannel(*out[i].Channel)
			out[i].Channel = &c
		}
		if out[i].Project != nil {
			p := cloneProject(*out[i].Project)
			out[i].Project = &p
		}
		if out[i].Upload != nil {
			f := publicFile(*out[i].Upload)
			out[i].Upload = &f
		}
	}
	return out, nil
}
