package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/subtle"
	"crypto/tls"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net"
	"net/http"
	netmail "net/mail"
	"net/smtp"
	"net/textproto"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/argon2"
)

const sessionCookieName = "infinitemail_session"
const oauthStateCookieName = "infinitemail_oauth_state"
const adminSessionMaxAge = 12 * time.Hour
const maxAttachmentCount = 50
const maxAttachmentBytes = 5 * 1024 * 1024
const maxMessageAttachmentBytes = 25 * 1024 * 1024

type response map[string]any

type apiHandler func(*http.Request) (int, any)

type App struct {
	store              *Store
	userAppOrigin      string
	adminAPIToken      string
	adminUsername      string
	adminPassword      string
	adminPasswordHash  string
	mailWebhookToken   string
	attachmentDir      string
	allowLocalSMSDebug bool
	adminSessionsMu    sync.Mutex
	adminSessions      map[string]AdminSessionRecord
}

type Store struct {
	mu    sync.Mutex
	path  string
	pg    *PostgresStateStore
	state AppState
}

type PostgresStateStore struct {
	pool *pgxpool.Pool
}

type AppState struct {
	Config   AdminConfig              `json:"config"`
	Users    []UserRecord             `json:"users"`
	Sessions map[string]SessionRecord `json:"sessions"`
	Settings map[string]MailSettings  `json:"settings"`
	Messages map[string][]MailMessage `json:"messages"`
}

type AdminSessionRecord struct {
	Token     string `json:"token"`
	Username  string `json:"username"`
	CreatedAt string `json:"createdAt"`
	ExpiresAt string `json:"expiresAt"`
}

type AdminConfig struct {
	Mailbox         MailboxConfig       `json:"mailbox"`
	Auth            AuthConfig          `json:"auth"`
	SMS             SMSConfig           `json:"sms"`
	Ops             OpsConfig           `json:"ops"`
	Security        AdminSecurityConfig `json:"security"`
	Deployment      DeploymentStatus    `json:"deployment"`
	Usage           UsageSnapshot       `json:"usage"`
	Invites         []InviteRecord      `json:"invites"`
	ProvisionJobs   []ProvisionJob      `json:"provisionJobs"`
	SMSLogs         []SMSLogRecord      `json:"smsLogs"`
	AuditLogs       []AuditLogRecord    `json:"auditLogs"`
	RegisteredUsers []RegisteredUser    `json:"registeredUsers"`
	UpdatedAt       string              `json:"updatedAt,omitempty"`
}

type MailboxConfig struct {
	Domain              string           `json:"domain"`
	PrefixPolicyEnabled bool             `json:"prefixPolicyEnabled"`
	AllowedPrefixes     []string         `json:"allowedPrefixes"`
	DefaultPrefix       string           `json:"defaultPrefix"`
	DNS                 DNSCheck         `json:"dns"`
	Server              MailServerConfig `json:"server"`
}

type DNSCheck struct {
	Status          string            `json:"status"`
	Domain          string            `json:"domain"`
	Selector        string            `json:"selector"`
	Records         []DNSRecordStatus `json:"records"`
	Recommended     []DNSRecordStatus `json:"recommended"`
	CheckedAt       string            `json:"checkedAt,omitempty"`
	NextCheckAfter  string            `json:"nextCheckAfter,omitempty"`
	VerifiedAt      string            `json:"verifiedAt,omitempty"`
	LastError       string            `json:"lastError,omitempty"`
	VerifiedRecords int               `json:"verifiedRecords"`
	TotalRecords    int               `json:"totalRecords"`
}

type DNSRecordStatus struct {
	Type     string `json:"type"`
	Host     string `json:"host"`
	Expected string `json:"expected"`
	Actual   string `json:"actual,omitempty"`
	Verified bool   `json:"verified"`
	Message  string `json:"message,omitempty"`
}

type MailServerConfig struct {
	Provider             string `json:"provider"`
	Enabled              bool   `json:"enabled"`
	StrictDataPlane      bool   `json:"strictDataPlane"`
	BaseURL              string `json:"baseUrl"`
	ProvisionPath        string `json:"provisionPath,omitempty"`
	LifecyclePath        string `json:"lifecyclePath,omitempty"`
	MessageListPath      string `json:"messageListPath,omitempty"`
	MessageDetailPath    string `json:"messageDetailPath,omitempty"`
	MessageSendPath      string `json:"messageSendPath,omitempty"`
	DraftPath            string `json:"draftPath,omitempty"`
	MessageStarPath      string `json:"messageStarPath,omitempty"`
	MessageMovePath      string `json:"messageMovePath,omitempty"`
	MessageReadPath      string `json:"messageReadPath,omitempty"`
	AdminToken           string `json:"adminToken,omitempty"`
	AdminTokenSet        bool   `json:"adminTokenSet"`
	AdminTokenMasked     string `json:"adminTokenMasked,omitempty"`
	SMTPEnabled          bool   `json:"smtpEnabled"`
	SMTPHost             string `json:"smtpHost,omitempty"`
	SMTPPort             int    `json:"smtpPort,omitempty"`
	SMTPUsername         string `json:"smtpUsername,omitempty"`
	SMTPPassword         string `json:"smtpPassword,omitempty"`
	SMTPPasswordSet      bool   `json:"smtpPasswordSet"`
	SMTPPasswordMasked   string `json:"smtpPasswordMasked,omitempty"`
	SMTPTLSMode          string `json:"smtpTlsMode,omitempty"`
	IMAPEnabled          bool   `json:"imapEnabled"`
	IMAPHost             string `json:"imapHost,omitempty"`
	IMAPPort             int    `json:"imapPort,omitempty"`
	IMAPUsername         string `json:"imapUsername,omitempty"`
	IMAPPassword         string `json:"imapPassword,omitempty"`
	IMAPPasswordSet      bool   `json:"imapPasswordSet"`
	IMAPPasswordMasked   string `json:"imapPasswordMasked,omitempty"`
	IMAPTLSMode          string `json:"imapTlsMode,omitempty"`
	Status               string `json:"status"`
	LastCheckedAt        string `json:"lastCheckedAt,omitempty"`
	LastError            string `json:"lastError,omitempty"`
	LastProvisionCheckAt string `json:"lastProvisionCheckAt,omitempty"`
	LastLifecycleSyncAt  string `json:"lastLifecycleSyncAt,omitempty"`
}

type AuthConfig struct {
	OAuthEnabled            bool     `json:"oauthEnabled"`
	OAuthProviderName       string   `json:"oauthProviderName"`
	OAuthClientID           string   `json:"oauthClientId,omitempty"`
	OAuthClientSecret       string   `json:"oauthClientSecret,omitempty"`
	OAuthClientSecretSet    bool     `json:"oauthClientSecretSet"`
	OAuthClientSecretMasked string   `json:"oauthClientSecretMasked,omitempty"`
	OAuthAuthorizeURL       string   `json:"oauthAuthorizeUrl,omitempty"`
	OAuthTokenURL           string   `json:"oauthTokenUrl,omitempty"`
	OAuthUserInfoURL        string   `json:"oauthUserInfoUrl,omitempty"`
	OAuthRedirectURL        string   `json:"oauthRedirectUrl,omitempty"`
	OAuthScopes             []string `json:"oauthScopes,omitempty"`
	OAuthSubjectField       string   `json:"oauthSubjectField,omitempty"`
	OAuthPhoneField         string   `json:"oauthPhoneField,omitempty"`
	OAuthEmailField         string   `json:"oauthEmailField,omitempty"`
	OAuthNameField          string   `json:"oauthNameField,omitempty"`
	PasswordLoginEnabled    bool     `json:"passwordLoginEnabled"`
	PhoneLoginEnabled       bool     `json:"phoneLoginEnabled"`
	EmailLoginEnabled       bool     `json:"emailLoginEnabled"`
	RegistrationEnabled     bool     `json:"registrationEnabled"`
	InviteRequired          bool     `json:"inviteRequired"`
	LoginLandingMode        string   `json:"loginLandingMode"`
}

type SMSConfig struct {
	Provider              string `json:"provider"`
	AliyunEnabled         bool   `json:"aliyunEnabled"`
	AccessKeyID           string `json:"accessKeyId"`
	AccessKeySecret       string `json:"accessKeySecret,omitempty"`
	AccessKeySecretSet    bool   `json:"accessKeySecretSet"`
	AccessKeySecretMasked string `json:"accessKeySecretMasked,omitempty"`
	SignName              string `json:"signName"`
	TemplateCode          string `json:"templateCode"`
	CodeTTLMinutes        int    `json:"codeTtlMinutes"`
}

type UsageSnapshot struct {
	ActiveSeats      int    `json:"activeSeats"`
	ReservedSeats    int    `json:"reservedSeats"`
	UsedSeats        int    `json:"usedSeats"`
	SeatLimit        int    `json:"seatLimit"`
	StorageUsedBytes int64  `json:"storageUsedBytes"`
	StorageUsedMB    int64  `json:"storageUsedMb"`
	StorageLimitGB   int    `json:"storageLimitGb"`
	StoragePercent   int    `json:"storagePercent"`
	UpdatedAt        string `json:"updatedAt,omitempty"`
}

type DeploymentStatus struct {
	Strict        bool              `json:"strict"`
	Ready         bool              `json:"ready"`
	Status        string            `json:"status"`
	Store         string            `json:"store"`
	UpdatedAt     string            `json:"updatedAt"`
	BlockingCount int               `json:"blockingCount"`
	Checks        []DeploymentCheck `json:"checks"`
}

type DeploymentCheck struct {
	ID       string `json:"id"`
	Label    string `json:"label"`
	Status   string `json:"status"`
	Required bool   `json:"required"`
	Message  string `json:"message"`
}

type ProvisionJob struct {
	ID          string `json:"id"`
	AccountID   string `json:"accountId"`
	Email       string `json:"email"`
	Status      string `json:"status"`
	Attempts    int    `json:"attempts"`
	LastError   string `json:"lastError,omitempty"`
	NextRunAt   string `json:"nextRunAt,omitempty"`
	LastRunAt   string `json:"lastRunAt,omitempty"`
	CreatedAt   string `json:"createdAt"`
	UpdatedAt   string `json:"updatedAt"`
	CompletedAt string `json:"completedAt,omitempty"`
}

type OperationalRunSummary struct {
	CheckedAt              string              `json:"checkedAt"`
	Provisioning           ProvisionRunSummary `json:"provisioning"`
	QueuedProvisionJobs    int                 `json:"queuedProvisionJobs"`
	FailedProvisionJobs    int                 `json:"failedProvisionJobs"`
	CompletedProvisionJobs int                 `json:"completedProvisionJobs"`
}

type OpsConfig struct {
	AutoRunEnabled  bool   `json:"autoRunEnabled"`
	IntervalMinutes int    `json:"intervalMinutes"`
	LastRunAt       string `json:"lastRunAt,omitempty"`
	LastRunStatus   string `json:"lastRunStatus,omitempty"`
	LastRunMessage  string `json:"lastRunMessage,omitempty"`
	UpdatedAt       string `json:"updatedAt,omitempty"`
}

type AdminSecurityConfig struct {
	Username          string `json:"username"`
	PasswordHash      string `json:"passwordHash,omitempty"`
	PasswordSet       bool   `json:"passwordSet"`
	PasswordUpdatedAt string `json:"passwordUpdatedAt,omitempty"`
	APITokenHash      string `json:"apiTokenHash,omitempty"`
	APITokenSet       bool   `json:"apiTokenSet"`
	APITokenMasked    string `json:"apiTokenMasked,omitempty"`
	APITokenUpdatedAt string `json:"apiTokenUpdatedAt,omitempty"`
	UpdatedAt         string `json:"updatedAt,omitempty"`
}

type ProvisionRunSummary struct {
	Processed int    `json:"processed"`
	Succeeded int    `json:"succeeded"`
	Failed    int    `json:"failed"`
	Skipped   int    `json:"skipped"`
	Message   string `json:"message"`
}

type MailboxProvisionPayload struct {
	ProvisionJobID string         `json:"provisionJobId"`
	AccountID      string         `json:"accountId"`
	Email          string         `json:"email"`
	LocalPart      string         `json:"localPart"`
	Domain         string         `json:"domain"`
	DisplayName    string         `json:"displayName"`
	Phone          string         `json:"phone"`
	Source         string         `json:"source"`
	Password       string         `json:"password"`
	QuotaBytes     int64          `json:"quotaBytes,omitempty"`
	Metadata       map[string]any `json:"metadata,omitempty"`
}

type MailboxProvisionResult struct {
	ExternalID      string `json:"externalId"`
	Status          string `json:"status"`
	Message         string `json:"message"`
	MailboxUsername string `json:"-"`
	MailboxPassword string `json:"-"`
}

type MailboxLifecyclePayload struct {
	Action      string         `json:"action"`
	AccountID   string         `json:"accountId"`
	Email       string         `json:"email"`
	LocalPart   string         `json:"localPart"`
	Domain      string         `json:"domain"`
	ExternalID  string         `json:"externalId,omitempty"`
	DisplayName string         `json:"displayName"`
	Phone       string         `json:"phone"`
	Password    string         `json:"password,omitempty"`
	Metadata    map[string]any `json:"metadata,omitempty"`
}

type MailboxLifecycleResult struct {
	ExternalID string `json:"externalId"`
	Status     string `json:"status"`
	Message    string `json:"message"`
}

type MessageBodyPayload struct {
	Format string `json:"format"`
	Text   string `json:"text"`
	HTML   string `json:"html"`
}

type SendMessagePayload struct {
	Recipients       []string           `json:"recipients"`
	CC               []string           `json:"cc"`
	BCC              []string           `json:"bcc"`
	Subject          string             `json:"subject"`
	Body             MessageBodyPayload `json:"body"`
	Attachments      []any              `json:"attachments"`
	TemplateID       string             `json:"templateId"`
	ReplyToMessageID string             `json:"replyToMessageId"`
	Source           string             `json:"source"`
}

type SaveDraftPayload struct {
	DraftID     string             `json:"draftId"`
	Recipients  []string           `json:"recipients"`
	CC          []string           `json:"cc"`
	BCC         []string           `json:"bcc"`
	Subject     string             `json:"subject"`
	Body        MessageBodyPayload `json:"body"`
	Attachments []any              `json:"attachments"`
	Autosave    bool               `json:"autosave"`
}

type MessageStarPatch struct {
	Starred *bool `json:"starred"`
}

type MessageMovePayload struct {
	TargetFolder string `json:"targetFolder"`
}

type MessageReadPatch struct {
	IsUnread *bool `json:"isUnread"`
	Read     *bool `json:"read"`
}

type MailboxMessageActionPayload struct {
	Action         string         `json:"action"`
	AccountID      string         `json:"accountId"`
	Email          string         `json:"email"`
	LocalPart      string         `json:"localPart"`
	Domain         string         `json:"domain"`
	MessageID      string         `json:"messageId"`
	Starred        *bool          `json:"starred,omitempty"`
	IsUnread       *bool          `json:"isUnread,omitempty"`
	Read           *bool          `json:"read,omitempty"`
	PreviousFolder string         `json:"previousFolder,omitempty"`
	TargetFolder   string         `json:"targetFolder,omitempty"`
	Metadata       map[string]any `json:"metadata,omitempty"`
}

type MailboxMessageRelayPayload struct {
	AccountID        string             `json:"accountId"`
	Email            string             `json:"email"`
	LocalPart        string             `json:"localPart"`
	Domain           string             `json:"domain"`
	DisplayName      string             `json:"displayName"`
	Phone            string             `json:"phone"`
	MessageID        string             `json:"messageId,omitempty"`
	DraftID          string             `json:"draftId,omitempty"`
	Recipients       []string           `json:"recipients"`
	CC               []string           `json:"cc"`
	BCC              []string           `json:"bcc"`
	Subject          string             `json:"subject"`
	Body             MessageBodyPayload `json:"body"`
	Attachments      []any              `json:"attachments"`
	Source           string             `json:"source"`
	ReplyToMessageID string             `json:"replyToMessageId,omitempty"`
	Autosave         bool               `json:"autosave,omitempty"`
	Metadata         map[string]any     `json:"metadata,omitempty"`
}

type MailboxMessageRelayResult struct {
	Message           MailMessage `json:"message"`
	Draft             MailMessage `json:"draft"`
	AcceptedAt        string      `json:"acceptedAt"`
	ProviderMessageID string      `json:"providerMessageId"`
	Status            string      `json:"status"`
	MessageText       string      `json:"messageText"`
}

type MailboxMessageListResult struct {
	Items      []MailMessage `json:"items"`
	HasMore    bool          `json:"hasMore"`
	NextCursor any           `json:"nextCursor"`
	Called     bool          `json:"-"`
}

type OAuthPrincipal struct {
	Subject     string         `json:"subject"`
	Phone       string         `json:"phone"`
	Email       string         `json:"email"`
	DisplayName string         `json:"displayName"`
	Raw         map[string]any `json:"raw,omitempty"`
}

type ContactRecord struct {
	ID              string       `json:"id"`
	Name            string       `json:"name"`
	Email           string       `json:"email"`
	Avatar          string       `json:"avatar"`
	Role            string       `json:"role"`
	Organization    string       `json:"organization"`
	LastContactedAt string       `json:"lastContactedAt"`
	Note            string       `json:"note"`
	Tags            []string     `json:"tags"`
	Stats           ContactStats `json:"stats"`
	SortAt          string       `json:"sortAt,omitempty"`
}

type ContactStats struct {
	TotalMessages int `json:"totalMessages"`
	InviteCount   int `json:"inviteCount"`
}

type ContactThreadItem struct {
	MailMessage
	FolderID string `json:"folderId"`
}

type MailTemplate struct {
	ID      string `json:"id"`
	Role    string `json:"role"`
	Subject string `json:"subject"`
	HTML    string `json:"html"`
}

type TemplateSendPayload struct {
	Role       string   `json:"role"`
	Recipients []string `json:"recipients"`
}

type InboundMailPayload struct {
	AccountID         string             `json:"accountId"`
	Email             string             `json:"email"`
	LocalPart         string             `json:"localPart"`
	Domain            string             `json:"domain"`
	MessageID         string             `json:"messageId"`
	ProviderMessageID string             `json:"providerMessageId"`
	ThreadID          string             `json:"threadId"`
	From              string             `json:"from"`
	Sender            string             `json:"sender"`
	SenderEmail       string             `json:"senderEmail"`
	To                []string           `json:"to"`
	Recipients        []string           `json:"recipients"`
	CC                []string           `json:"cc"`
	BCC               []string           `json:"bcc"`
	Subject           string             `json:"subject"`
	Snippet           string             `json:"snippet"`
	Text              string             `json:"text"`
	HTML              string             `json:"html"`
	Body              MessageBodyPayload `json:"body"`
	Attachments       []any              `json:"attachments"`
	ReceivedAt        string             `json:"receivedAt"`
	Headers           map[string]string  `json:"headers"`
	Raw               json.RawMessage    `json:"message"`
}

type DeliveryStatusPayload struct {
	MessageID         string `json:"messageId"`
	ProviderMessageID string `json:"providerMessageId"`
	Status            string `json:"status"`
	AcceptedAt        string `json:"acceptedAt"`
	Error             string `json:"error"`
	DeliveryError     string `json:"deliveryError"`
}

type AttachmentAssetRecord struct {
	ID          string `json:"id"`
	AccountID   string `json:"accountId"`
	Name        string `json:"name"`
	Type        string `json:"type"`
	ContentType string `json:"contentType"`
	SizeBytes   int64  `json:"sizeBytes"`
	SHA256      string `json:"sha256"`
	CreatedAt   string `json:"createdAt"`
}

type InviteRecord struct {
	ID               string `json:"id"`
	Code             string `json:"code"`
	Email            string `json:"email"`
	MailboxLocalPart string `json:"mailboxLocalPart"`
	Phone            string `json:"phone"`
	Note             string `json:"note"`
	CreatedAt        string `json:"createdAt"`
	ExpiresAt        string `json:"expiresAt"`
	UsedAt           string `json:"usedAt"`
	URL              string `json:"url"`
}

type SMSLogRecord struct {
	ID         string `json:"id"`
	Phone      string `json:"phone"`
	Code       string `json:"code,omitempty"`
	CodeHash   string `json:"codeHash,omitempty"`
	CodeMasked string `json:"codeMasked,omitempty"`
	Purpose    string `json:"purpose"`
	Provider   string `json:"provider"`
	Status     string `json:"status"`
	CreatedAt  string `json:"createdAt"`
	ExpiresAt  string `json:"expiresAt"`
}

type AuditLogRecord struct {
	ID        string `json:"id"`
	Actor     string `json:"actor"`
	Action    string `json:"action"`
	Target    string `json:"target"`
	Detail    string `json:"detail"`
	IP        string `json:"ip"`
	CreatedAt string `json:"createdAt"`
}

type RegisteredUser struct {
	ID                   string `json:"id"`
	Phone                string `json:"phone"`
	Email                string `json:"email"`
	DisplayName          string `json:"displayName"`
	RegisteredAt         string `json:"registeredAt"`
	Source               string `json:"source"`
	Status               string `json:"status"`
	DisabledAt           string `json:"disabledAt,omitempty"`
	LastPasswordResetAt  string `json:"lastPasswordResetAt,omitempty"`
	MailboxStatus        string `json:"mailboxStatus"`
	MailboxProvisionedAt string `json:"mailboxProvisionedAt,omitempty"`
	MailboxExternalID    string `json:"mailboxExternalId,omitempty"`
	MailboxLastError     string `json:"mailboxLastError,omitempty"`
}

type UserRecord struct {
	RegisteredUser
	PasswordHash          string `json:"passwordHash"`
	MailboxUsername       string `json:"mailboxUsername,omitempty"`
	MailboxPasswordSecret string `json:"mailboxPasswordSecret,omitempty"`
}

type SessionRecord struct {
	ID         string `json:"id"`
	Token      string `json:"token"`
	UserID     string `json:"userId"`
	IP         string `json:"ip"`
	UserAgent  string `json:"userAgent"`
	Device     string `json:"device"`
	CreatedAt  string `json:"createdAt"`
	LastSeenAt string `json:"lastSeenAt"`
	ExpiresAt  string `json:"expiresAt"`
}

type MailSettings struct {
	DefaultSenderName string `json:"defaultSenderName"`
	Signature         string `json:"signature"`
	AutoReplyEnabled  bool   `json:"autoReplyEnabled"`
	AutoReplyMessage  string `json:"autoReplyMessage"`
	UpdatedAt         string `json:"updatedAt,omitempty"`
}

type mailSettingsPatch struct {
	DefaultSenderName *string `json:"defaultSenderName"`
	Signature         *string `json:"signature"`
	AutoReplyEnabled  *bool   `json:"autoReplyEnabled"`
	AutoReplyMessage  *string `json:"autoReplyMessage"`
}

type MailProfile struct {
	ID                  string `json:"id"`
	DisplayName         string `json:"displayName"`
	AvatarInitial       string `json:"avatarInitial"`
	UnifiedAccountPhone string `json:"unifiedAccountPhone"`
	RolePrefix          string `json:"rolePrefix"`
	EmailPrefix         string `json:"emailPrefix"`
	Email               string `json:"email"`
	MailboxDomain       string `json:"mailboxDomain"`
	MailboxProvisioned  bool   `json:"mailboxProvisioned"`
	ProvisioningStatus  string `json:"provisioningStatus"`
	AuthMode            string `json:"authMode"`
	SourceUserID        string `json:"sourceUserId"`
	CreatedAt           string `json:"createdAt,omitempty"`
	UpdatedAt           string `json:"updatedAt,omitempty"`
	ProvisionedAt       string `json:"provisionedAt,omitempty"`
}

type MailMessage struct {
	ID                string   `json:"id"`
	ThreadID          string   `json:"threadId,omitempty"`
	Folder            string   `json:"folder"`
	PreviousFolder    string   `json:"previousFolder"`
	Sender            string   `json:"sender"`
	SenderEmail       string   `json:"senderEmail"`
	Recipients        []string `json:"recipients"`
	Avatar            string   `json:"avatar"`
	Role              string   `json:"role"`
	Subject           string   `json:"subject"`
	Snippet           string   `json:"snippet"`
	Time              string   `json:"time"`
	DateTimeLabel     string   `json:"dateTimeLabel"`
	SortAt            string   `json:"sortAt"`
	SentAt            string   `json:"sentAt,omitempty"`
	ReceivedAt        string   `json:"receivedAt,omitempty"`
	IsUnread          bool     `json:"isUnread"`
	IsStarred         bool     `json:"isStarred"`
	HasAttachment     bool     `json:"hasAttachment"`
	Tags              []string `json:"tags"`
	IsOutgoing        bool     `json:"isOutgoing"`
	Content           string   `json:"content"`
	Attachments       []any    `json:"attachments"`
	Source            string   `json:"source"`
	DeliveryStatus    string   `json:"deliveryStatus"`
	AcceptedAt        string   `json:"acceptedAt,omitempty"`
	ProviderMessageID string   `json:"providerMessageId,omitempty"`
	DeliveryError     string   `json:"deliveryError,omitempty"`
}

type mailboxPatch struct {
	Domain              *string          `json:"domain"`
	PrefixPolicyEnabled *bool            `json:"prefixPolicyEnabled"`
	AllowedPrefixes     []string         `json:"allowedPrefixes"`
	DefaultPrefix       *string          `json:"defaultPrefix"`
	Server              *mailServerPatch `json:"server"`
}

type mailServerPatch struct {
	Provider          *string `json:"provider"`
	Enabled           *bool   `json:"enabled"`
	StrictDataPlane   *bool   `json:"strictDataPlane"`
	BaseURL           *string `json:"baseUrl"`
	ProvisionPath     *string `json:"provisionPath"`
	LifecyclePath     *string `json:"lifecyclePath"`
	MessageListPath   *string `json:"messageListPath"`
	MessageDetailPath *string `json:"messageDetailPath"`
	MessageSendPath   *string `json:"messageSendPath"`
	DraftPath         *string `json:"draftPath"`
	MessageStarPath   *string `json:"messageStarPath"`
	MessageMovePath   *string `json:"messageMovePath"`
	MessageReadPath   *string `json:"messageReadPath"`
	AdminToken        *string `json:"adminToken"`
	SMTPEnabled       *bool   `json:"smtpEnabled"`
	SMTPHost          *string `json:"smtpHost"`
	SMTPPort          *int    `json:"smtpPort"`
	SMTPUsername      *string `json:"smtpUsername"`
	SMTPPassword      *string `json:"smtpPassword"`
	SMTPTLSMode       *string `json:"smtpTlsMode"`
	IMAPEnabled       *bool   `json:"imapEnabled"`
	IMAPHost          *string `json:"imapHost"`
	IMAPPort          *int    `json:"imapPort"`
	IMAPUsername      *string `json:"imapUsername"`
	IMAPPassword      *string `json:"imapPassword"`
	IMAPTLSMode       *string `json:"imapTlsMode"`
}

type authPatch struct {
	OAuthEnabled         *bool    `json:"oauthEnabled"`
	OAuthProviderName    *string  `json:"oauthProviderName"`
	OAuthClientID        *string  `json:"oauthClientId"`
	OAuthClientSecret    *string  `json:"oauthClientSecret"`
	OAuthAuthorizeURL    *string  `json:"oauthAuthorizeUrl"`
	OAuthTokenURL        *string  `json:"oauthTokenUrl"`
	OAuthUserInfoURL     *string  `json:"oauthUserInfoUrl"`
	OAuthRedirectURL     *string  `json:"oauthRedirectUrl"`
	OAuthScopes          []string `json:"oauthScopes"`
	OAuthSubjectField    *string  `json:"oauthSubjectField"`
	OAuthPhoneField      *string  `json:"oauthPhoneField"`
	OAuthEmailField      *string  `json:"oauthEmailField"`
	OAuthNameField       *string  `json:"oauthNameField"`
	PasswordLoginEnabled *bool    `json:"passwordLoginEnabled"`
	PhoneLoginEnabled    *bool    `json:"phoneLoginEnabled"`
	EmailLoginEnabled    *bool    `json:"emailLoginEnabled"`
	RegistrationEnabled  *bool    `json:"registrationEnabled"`
	InviteRequired       *bool    `json:"inviteRequired"`
	LoginLandingMode     *string  `json:"loginLandingMode"`
}

type smsPatch struct {
	Provider        *string `json:"provider"`
	AliyunEnabled   *bool   `json:"aliyunEnabled"`
	AccessKeyID     *string `json:"accessKeyId"`
	AccessKeySecret *string `json:"accessKeySecret"`
	SignName        *string `json:"signName"`
	TemplateCode    *string `json:"templateCode"`
	CodeTTLMinutes  *int    `json:"codeTtlMinutes"`
}

type opsPatch struct {
	AutoRunEnabled  *bool `json:"autoRunEnabled"`
	IntervalMinutes *int  `json:"intervalMinutes"`
}

type adminSecurityPatch struct {
	Username      *string `json:"username"`
	NewPassword   *string `json:"newPassword"`
	APIToken      *string `json:"apiToken"`
	ClearAPIToken *bool   `json:"clearApiToken"`
}

type adminConfigPatch struct {
	Mailbox  *mailboxPatch       `json:"mailbox"`
	Auth     *authPatch          `json:"auth"`
	SMS      *smsPatch           `json:"sms"`
	Ops      *opsPatch           `json:"ops"`
	Security *adminSecurityPatch `json:"security"`
}

var (
	phoneRe         = regexp.MustCompile(`^1\d{10}$`)
	localPartRe     = regexp.MustCompile(`[^a-z0-9._-]+`)
	prefixRe        = regexp.MustCompile(`[^a-z0-9]+`)
	assetIDRe       = regexp.MustCompile(`^[A-Za-z0-9._-]{6,120}$`)
	sessionMaxAge   = 30 * 24 * time.Hour
	errUnauthorized = errors.New("unauthorized")
)

func main() {
	addr := env("HTTP_ADDR", ":1666")
	app, err := newApp()
	if err != nil {
		slog.Error("BFF init failed", "error", err)
		os.Exit(1)
	}

	mux := http.NewServeMux()
	app.routes(mux)
	workerCtx, stopWorker := context.WithCancel(context.Background())
	defer stopWorker()
	app.startOpsWorker(workerCtx)

	server := &http.Server{
		Addr:              addr,
		Handler:           withCORS(withRequestLog(mux)),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	slog.Info("InfiniteMail BFF started", "addr", addr)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		slog.Error("server stopped", "error", err)
		os.Exit(1)
	}
}

func newApp() (*App, error) {
	var store *Store
	var err error
	if databaseURL := strings.TrimSpace(env("DATABASE_URL", "")); databaseURL != "" {
		store, err = newPostgresStore(
			context.Background(),
			databaseURL,
			env("MIGRATIONS_DIR", "migrations"),
		)
	} else {
		store, err = newStore(env("DATA_PATH", filepath.Join(".data", "infinitemail-bff.json")))
	}
	if err != nil {
		return nil, err
	}
	return &App{
		store:              store,
		userAppOrigin:      strings.TrimRight(env("USER_APP_ORIGIN", "http://127.0.0.1:1788"), "/"),
		adminAPIToken:      strings.TrimSpace(env("ADMIN_API_TOKEN", "")),
		adminUsername:      firstNonEmpty(strings.TrimSpace(env("ADMIN_USERNAME", "")), "admin"),
		adminPassword:      strings.TrimSpace(env("ADMIN_PASSWORD", "")),
		adminPasswordHash:  strings.TrimSpace(env("ADMIN_PASSWORD_HASH", "")),
		mailWebhookToken:   strings.TrimSpace(env("MAIL_WEBHOOK_TOKEN", "")),
		attachmentDir:      env("ATTACHMENT_DIR", filepath.Join(".data", "attachments")),
		allowLocalSMSDebug: localSMSDebugAllowedFromEnv(),
		adminSessions:      map[string]AdminSessionRecord{},
	}, nil
}

func (app *App) routes(mux *http.ServeMux) {
	handle(mux, "GET", "/healthz", app.health)
	handle(mux, "GET", "/readyz", app.ready)
	handle(mux, "GET", "/api/healthz", app.health)

	app.handleAPI(mux, "GET", "/auth/session", app.getSession)
	app.handleAPI(mux, "POST", "/auth/oauth/start", app.beginOAuth)
	app.handleRawAPI(mux, "GET", "/auth/oauth/callback", app.finishOAuth)
	app.handleAPI(mux, "POST", "/auth/sms/send", app.sendSMSCode)
	app.handleAPI(mux, "POST", "/auth/register", app.registerWithPassword)
	app.handleAPI(mux, "POST", "/auth/login", app.loginWithPassword)
	app.handleAPI(mux, "POST", "/auth/logout", app.logout)
	app.handleAPI(mux, "GET", "/security/sessions", app.listSecuritySessions)
	app.handleAPI(mux, "POST", "/security/sessions/logout-others", app.logoutOtherSecuritySessions)
	app.handleAPI(mux, "POST", "/security/sessions/{sessionID}/revoke", app.revokeSecuritySession)

	app.handleAPI(mux, "GET", "/admin/auth/session", app.getAdminAuthSession)
	app.handleAPI(mux, "POST", "/admin/auth/login", app.loginAdmin)
	app.handleAPI(mux, "POST", "/admin/auth/logout", app.logoutAdmin)

	app.handleAdminAPI(mux, "GET", "/admin/mail/config", app.getAdminConfig)
	app.handleAdminAPI(mux, "PATCH", "/admin/mail/config", app.updateAdminConfig)
	app.handleAdminAPI(mux, "POST", "/admin/mail/domains/verify", app.verifyMailboxDomain)
	app.handleAdminAPI(mux, "POST", "/admin/mail/server/test", app.testMailServer)
	app.handleAdminAPI(mux, "GET", "/admin/mail/invites", app.listInvites)
	app.handleAdminAPI(mux, "POST", "/admin/mail/invites", app.createInvite)
	app.handleAdminAPI(mux, "GET", "/admin/mail/accounts", app.listAccounts)
	app.handleAdminAPI(mux, "POST", "/admin/mail/accounts/{accountID}/disable", app.disableAccount)
	app.handleAdminAPI(mux, "POST", "/admin/mail/accounts/{accountID}/enable", app.enableAccount)
	app.handleAdminAPI(mux, "POST", "/admin/mail/accounts/{accountID}/reset-password", app.resetAccountPassword)
	app.handleAdminAPI(mux, "POST", "/admin/mail/accounts/{accountID}/provision", app.retryMailboxProvision)
	app.handleAdminAPI(mux, "POST", "/admin/mail/ops/run", app.runOperationalTasks)
	app.handleAdminAPI(mux, "GET", "/admin/mail/sms-logs", app.listSMSLogs)
	app.handleAdminAPI(mux, "GET", "/admin/mail/audit-logs", app.listAuditLogs)

	app.handleAPI(mux, "GET", "/mailboxes/me", app.getMailboxProfile)
	app.handleAPI(mux, "POST", "/mailboxes/activate", app.activateMailbox)
	app.handleAPI(mux, "GET", "/messages", app.listMessages)
	app.handleAPI(mux, "GET", "/messages/{messageID}", app.getMessageDetail)
	app.handleAPI(mux, "POST", "/messages/send", app.sendMessage)
	app.handleAPI(mux, "PATCH", "/messages/{messageID}/star", app.updateMessageStar)
	app.handleAPI(mux, "POST", "/messages/{messageID}/move", app.moveMessage)
	app.handleAPI(mux, "PATCH", "/messages/{messageID}/read", app.updateMessageReadState)
	app.handleAPI(mux, "POST", "/drafts", app.saveDraft)
	app.handleAPI(mux, "POST", "/attachments", app.uploadAttachments)
	app.handleRawAPI(mux, "GET", "/attachments/{assetID}/download", app.downloadAttachment)
	app.handleAPI(mux, "GET", "/contacts", app.listContacts)
	app.handleAPI(mux, "GET", "/contacts/{contactID}/thread", app.getContactThread)
	app.handleAPI(mux, "GET", "/templates", app.listTemplates)
	app.handleAPI(mux, "POST", "/templates/send", app.sendTemplateMessage)
	app.handleAPI(mux, "POST", "/webhooks/mail/inbound", app.ingestInboundMail)
	app.handleAPI(mux, "POST", "/webhooks/mail/delivery", app.updateDeliveryStatus)
	app.handleAPI(mux, "POST", "/mail/inbound", app.ingestInboundMail)
	app.handleAPI(mux, "POST", "/mail/delivery", app.updateDeliveryStatus)
	app.handleAPI(mux, "GET", "/settings", app.getSettings)
	app.handleAPI(mux, "PUT", "/settings", app.updateSettings)
}

func (app *App) startOpsWorker(ctx context.Context) {
	tickerInterval := opsWorkerTickInterval()
	go func() {
		app.runDueOpsWorker()
		ticker := time.NewTicker(tickerInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				app.runDueOpsWorker()
			}
		}
	}()
}

func (app *App) runDueOpsWorker() {
	now := time.Now()
	snapshot := app.store.snapshot()
	if !opsAutoRunDue(snapshot.Config.Ops, now) {
		return
	}
	result, err := app.store.mutate(func(state *AppState) (any, error) {
		now := time.Now()
		state.Config.Ops = normalizeOpsConfig(state.Config.Ops)
		if !opsAutoRunDue(state.Config.Ops, now) {
			return nil, nil
		}
		summary := runOperationalTasksState(context.Background(), state, now)
		applyOperationalRunMetadata(&state.Config, summary)
		state.Config.UpdatedAt = summary.CheckedAt
		appendAuditLog(state, nil, "system", "system.ops.auto_run", "ops", summaryMessage(summary))
		return summary, nil
	})
	if err != nil {
		slog.Warn("ops worker failed", "error", err)
		return
	}
	if summary, ok := result.(OperationalRunSummary); ok {
		slog.Info("ops worker completed", "status", stateSummaryStatus(summary), "message", summaryMessage(summary))
	}
}

func opsWorkerTickInterval() time.Duration {
	seconds, err := strconv.Atoi(strings.TrimSpace(env("OPS_WORKER_TICK_SECONDS", "")))
	if err == nil && seconds > 0 {
		return time.Duration(seconds) * time.Second
	}
	return 30 * time.Second
}

func (app *App) handleAPI(mux *http.ServeMux, method string, path string, handler apiHandler) {
	for _, base := range []string{"/api/v1/post-office", "/api"} {
		handle(mux, method, base+path, handler)
	}
}

func (app *App) handleRawAPI(mux *http.ServeMux, method string, path string, handler http.HandlerFunc) {
	for _, base := range []string{"/api/v1/post-office", "/api"} {
		mux.HandleFunc(method+" "+base+path, handler)
	}
}

func (app *App) apiBasePath(r *http.Request) string {
	path := strings.TrimSpace(r.URL.Path)
	if strings.HasPrefix(path, "/api/v1/post-office/") || path == "/api/v1/post-office" {
		return "/api/v1/post-office"
	}
	return "/api"
}

func (app *App) handleAdminAPI(mux *http.ServeMux, method string, path string, handler apiHandler) {
	app.handleAPI(mux, method, path, app.requireAdmin(handler))
}

func (app *App) requireAdmin(next apiHandler) apiHandler {
	return func(r *http.Request) (int, any) {
		if app.authorizedAdmin(r) {
			return next(r)
		}
		return http.StatusUnauthorized, response{"message": "后台接口未授权"}
	}
}

func (app *App) adminAuthRequired() bool {
	security := app.storedAdminSecurity()
	return security.PasswordSet || security.APITokenSet ||
		strings.TrimSpace(app.adminAPIToken) != "" ||
		strings.TrimSpace(app.adminPassword) != "" ||
		strings.TrimSpace(app.adminPasswordHash) != ""
}

func (app *App) authorizedAdmin(r *http.Request) bool {
	if !app.adminAuthRequired() {
		return true
	}
	token := adminTokenFromRequest(r)
	if app.adminAPIToken != "" && constantTimeStringEqual(token, app.adminAPIToken) {
		return true
	}
	security := app.storedAdminSecurity()
	if verifyAdminAPITokenHash(security.APITokenHash, token) {
		return true
	}
	session, ok := app.lookupAdminSession(token)
	return ok && !adminSessionExpired(session)
}

func (app *App) authorizedMailWebhook(r *http.Request) bool {
	token := strings.TrimSpace(app.mailWebhookToken)
	if token == "" {
		return false
	}
	candidates := []string{
		strings.TrimSpace(r.Header.Get("X-Mail-Webhook-Token")),
		strings.TrimSpace(r.URL.Query().Get("token")),
	}
	header := strings.TrimSpace(r.Header.Get("Authorization"))
	if strings.HasPrefix(strings.ToLower(header), "bearer ") {
		candidates = append(candidates, strings.TrimSpace(header[7:]))
	}
	for _, candidate := range candidates {
		if candidate != "" && constantTimeStringEqual(candidate, token) {
			return true
		}
	}
	return false
}

func handle(mux *http.ServeMux, method string, path string, handler apiHandler) {
	mux.HandleFunc(method+" "+path, jsonHandler(handler))
}

func newStore(path string) (*Store, error) {
	store := &Store{
		path: path,
		state: AppState{
			Config:   defaultAdminConfig(),
			Sessions: map[string]SessionRecord{},
			Settings: map[string]MailSettings{},
			Messages: map[string][]MailMessage{},
		},
	}

	if raw, err := os.ReadFile(path); err == nil && len(raw) > 0 {
		if err := json.Unmarshal(raw, &store.state); err != nil {
			return nil, fmt.Errorf("load store: %w", err)
		}
	} else if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("read store: %w", err)
	}

	store.normalizeLocked()
	return store, nil
}

func newPostgresStore(ctx context.Context, databaseURL string, migrationsDir string) (*Store, error) {
	pgStore, err := newPostgresStateStore(ctx, databaseURL, migrationsDir)
	if err != nil {
		return nil, err
	}

	store := &Store{
		pg: pgStore,
		state: AppState{
			Config:   defaultAdminConfig(),
			Sessions: map[string]SessionRecord{},
			Settings: map[string]MailSettings{},
			Messages: map[string][]MailMessage{},
		},
	}

	loaded, ok, err := pgStore.load(ctx)
	if err != nil {
		pgStore.close()
		return nil, err
	}
	if ok {
		store.state = loaded
	}
	store.normalizeLocked()
	if !ok {
		if err := store.saveLocked(); err != nil {
			pgStore.close()
			return nil, err
		}
	}
	return store, nil
}

func newPostgresStateStore(ctx context.Context, databaseURL string, migrationsDir string) (*PostgresStateStore, error) {
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, fmt.Errorf("postgres pool: %w", err)
	}
	store := &PostgresStateStore{pool: pool}
	if err := pool.Ping(ctx); err != nil {
		store.close()
		return nil, fmt.Errorf("postgres ping: %w", err)
	}
	if err := store.migrate(ctx, migrationsDir); err != nil {
		store.close()
		return nil, err
	}
	return store, nil
}

func (p *PostgresStateStore) migrate(ctx context.Context, migrationsDir string) error {
	if strings.TrimSpace(migrationsDir) == "" {
		migrationsDir = "migrations"
	}
	if _, err := p.pool.Exec(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (version text PRIMARY KEY, applied_at timestamptz NOT NULL DEFAULT now())`); err != nil {
		return fmt.Errorf("ensure schema migrations: %w", err)
	}

	files, err := filepath.Glob(filepath.Join(migrationsDir, "*.sql"))
	if err != nil {
		return fmt.Errorf("list migrations: %w", err)
	}
	sort.Strings(files)
	for _, file := range files {
		version := filepath.Base(file)
		var applied bool
		if err := p.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM schema_migrations WHERE version = $1)`, version).Scan(&applied); err != nil {
			return fmt.Errorf("check migration %s: %w", version, err)
		}
		if applied {
			continue
		}
		sql, err := os.ReadFile(file)
		if err != nil {
			return fmt.Errorf("read migration %s: %w", version, err)
		}
		if len(strings.TrimSpace(string(sql))) == 0 {
			continue
		}
		tx, err := p.pool.Begin(ctx)
		if err != nil {
			return fmt.Errorf("begin migration %s: %w", version, err)
		}
		if _, err := tx.Exec(ctx, string(sql)); err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("apply migration %s: %w", version, err)
		}
		if _, err := tx.Exec(ctx, `INSERT INTO schema_migrations (version) VALUES ($1)`, version); err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("record migration %s: %w", version, err)
		}
		if err := tx.Commit(ctx); err != nil {
			return fmt.Errorf("commit migration %s: %w", version, err)
		}
	}
	return nil
}

func (p *PostgresStateStore) load(ctx context.Context) (AppState, bool, error) {
	state, ok, err := p.loadNormalized(ctx)
	if err != nil {
		return AppState{}, false, err
	}
	if ok {
		return state, true, nil
	}
	return p.loadSnapshot(ctx)
}

func (p *PostgresStateStore) loadSnapshot(ctx context.Context) (AppState, bool, error) {
	var raw []byte
	err := p.pool.QueryRow(ctx, `SELECT payload FROM bff_state_snapshots WHERE id = 'default'`).Scan(&raw)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return AppState{}, false, nil
		}
		return AppState{}, false, fmt.Errorf("load postgres state: %w", err)
	}
	var state AppState
	if err := json.Unmarshal(raw, &state); err != nil {
		return AppState{}, false, fmt.Errorf("decode postgres state: %w", err)
	}
	return state, true, nil
}

func (p *PostgresStateStore) save(ctx context.Context, state AppState) error {
	if err := p.saveNormalized(ctx, state); err != nil {
		return err
	}
	return p.saveSnapshot(ctx, state)
}

func (p *PostgresStateStore) saveSnapshot(ctx context.Context, state AppState) error {
	raw, err := json.Marshal(state)
	if err != nil {
		return err
	}
	_, err = p.pool.Exec(ctx, `
		INSERT INTO bff_state_snapshots (id, payload, updated_at)
		VALUES ('default', $1, now())
		ON CONFLICT (id) DO UPDATE SET payload = EXCLUDED.payload, updated_at = now()
	`, raw)
	if err != nil {
		return fmt.Errorf("save postgres state: %w", err)
	}
	return nil
}

func (p *PostgresStateStore) loadNormalized(ctx context.Context) (AppState, bool, error) {
	state := AppState{
		Config:   defaultAdminConfig(),
		Sessions: map[string]SessionRecord{},
		Settings: map[string]MailSettings{},
		Messages: map[string][]MailMessage{},
	}

	var mailboxRaw []byte
	var authRaw []byte
	var smsRaw []byte
	var opsRaw []byte
	var securityRaw []byte
	var updatedAt time.Time
	err := p.pool.QueryRow(ctx, `
		SELECT mailbox, auth, sms, ops, security, updated_at
		FROM admin_mail_config
		WHERE id = true
	`).Scan(&mailboxRaw, &authRaw, &smsRaw, &opsRaw, &securityRaw, &updatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return AppState{}, false, nil
		}
		return AppState{}, false, fmt.Errorf("load admin config: %w", err)
	}
	if len(mailboxRaw) > 0 {
		if err := json.Unmarshal(mailboxRaw, &state.Config.Mailbox); err != nil {
			return AppState{}, false, fmt.Errorf("decode mailbox config: %w", err)
		}
	}
	if len(authRaw) > 0 {
		if err := json.Unmarshal(authRaw, &state.Config.Auth); err != nil {
			return AppState{}, false, fmt.Errorf("decode auth config: %w", err)
		}
	}
	if len(smsRaw) > 0 {
		if err := json.Unmarshal(smsRaw, &state.Config.SMS); err != nil {
			return AppState{}, false, fmt.Errorf("decode sms config: %w", err)
		}
	}
	if len(opsRaw) > 0 {
		if err := json.Unmarshal(opsRaw, &state.Config.Ops); err != nil {
			return AppState{}, false, fmt.Errorf("decode ops config: %w", err)
		}
	}
	if len(securityRaw) > 0 {
		if err := json.Unmarshal(securityRaw, &state.Config.Security); err != nil {
			return AppState{}, false, fmt.Errorf("decode security config: %w", err)
		}
	}
	state.Config.UpdatedAt = timeToISO(updatedAt)

	inviteRows, err := p.pool.Query(ctx, `
		SELECT id, code, email::text, mailbox_local_part, COALESCE(phone, ''), note, created_at, expires_at, used_at, url
		FROM mailbox_invites
		ORDER BY created_at DESC
		LIMIT 100
	`)
	if err != nil {
		return AppState{}, false, fmt.Errorf("load invites: %w", err)
	}
	defer inviteRows.Close()
	for inviteRows.Next() {
		var invite InviteRecord
		var createdAt time.Time
		var expiresAt sql.NullTime
		var usedAt sql.NullTime
		if err := inviteRows.Scan(&invite.ID, &invite.Code, &invite.Email, &invite.MailboxLocalPart, &invite.Phone, &invite.Note, &createdAt, &expiresAt, &usedAt, &invite.URL); err != nil {
			return AppState{}, false, fmt.Errorf("scan invite: %w", err)
		}
		invite.CreatedAt = timeToISO(createdAt)
		invite.ExpiresAt = nullTimeToISO(expiresAt)
		invite.UsedAt = nullTimeToISO(usedAt)
		state.Config.Invites = append(state.Config.Invites, invite)
	}
	if err := inviteRows.Err(); err != nil {
		return AppState{}, false, fmt.Errorf("iterate invites: %w", err)
	}

	smsRows, err := p.pool.Query(ctx, `
		SELECT id, phone, code, code_hash, code_masked, purpose, provider, status, created_at, expires_at
		FROM sms_code_logs
		ORDER BY created_at DESC
		LIMIT 200
	`)
	if err != nil {
		return AppState{}, false, fmt.Errorf("load sms logs: %w", err)
	}
	defer smsRows.Close()
	for smsRows.Next() {
		var log SMSLogRecord
		var createdAt time.Time
		var expiresAt time.Time
		if err := smsRows.Scan(&log.ID, &log.Phone, &log.Code, &log.CodeHash, &log.CodeMasked, &log.Purpose, &log.Provider, &log.Status, &createdAt, &expiresAt); err != nil {
			return AppState{}, false, fmt.Errorf("scan sms log: %w", err)
		}
		log.CreatedAt = timeToISO(createdAt)
		log.ExpiresAt = timeToISO(expiresAt)
		state.Config.SMSLogs = append(state.Config.SMSLogs, log)
	}
	if err := smsRows.Err(); err != nil {
		return AppState{}, false, fmt.Errorf("iterate sms logs: %w", err)
	}

	auditRows, err := p.pool.Query(ctx, `
		SELECT id, actor, action, target, detail, ip, created_at
		FROM audit_logs
		ORDER BY created_at DESC
		LIMIT 500
	`)
	if err != nil {
		return AppState{}, false, fmt.Errorf("load audit logs: %w", err)
	}
	defer auditRows.Close()
	for auditRows.Next() {
		var log AuditLogRecord
		var createdAt time.Time
		if err := auditRows.Scan(&log.ID, &log.Actor, &log.Action, &log.Target, &log.Detail, &log.IP, &createdAt); err != nil {
			return AppState{}, false, fmt.Errorf("scan audit log: %w", err)
		}
		log.CreatedAt = timeToISO(createdAt)
		state.Config.AuditLogs = append(state.Config.AuditLogs, log)
	}
	if err := auditRows.Err(); err != nil {
		return AppState{}, false, fmt.Errorf("iterate audit logs: %w", err)
	}

	accountRows, err := p.pool.Query(ctx, `
		SELECT id, phone, email::text, display_name, password_hash, source, registered_at,
			COALESCE(status, 'active'), disabled_at, password_reset_at,
			COALESCE(mailbox_status, 'pending_config'), mailbox_provisioned_at, mailbox_external_id, mailbox_last_error,
			COALESCE(mailbox_username, ''), COALESCE(mailbox_password_secret, '')
		FROM mail_accounts
		ORDER BY registered_at DESC
	`)
	if err != nil {
		return AppState{}, false, fmt.Errorf("load accounts: %w", err)
	}
	defer accountRows.Close()
	for accountRows.Next() {
		var user UserRecord
		var registeredAt time.Time
		var disabledAt sql.NullTime
		var passwordResetAt sql.NullTime
		var mailboxProvisionedAt sql.NullTime
		if err := accountRows.Scan(
			&user.ID,
			&user.Phone,
			&user.Email,
			&user.DisplayName,
			&user.PasswordHash,
			&user.Source,
			&registeredAt,
			&user.Status,
			&disabledAt,
			&passwordResetAt,
			&user.MailboxStatus,
			&mailboxProvisionedAt,
			&user.MailboxExternalID,
			&user.MailboxLastError,
			&user.MailboxUsername,
			&user.MailboxPasswordSecret,
		); err != nil {
			return AppState{}, false, fmt.Errorf("scan account: %w", err)
		}
		user.RegisteredAt = timeToISO(registeredAt)
		user.DisabledAt = nullTimeToISO(disabledAt)
		user.LastPasswordResetAt = nullTimeToISO(passwordResetAt)
		user.MailboxProvisionedAt = nullTimeToISO(mailboxProvisionedAt)
		state.Users = append(state.Users, user)
		state.Config.RegisteredUsers = append(state.Config.RegisteredUsers, user.RegisteredUser)
	}
	if err := accountRows.Err(); err != nil {
		return AppState{}, false, fmt.Errorf("iterate accounts: %w", err)
	}

	provisionRows, err := p.pool.Query(ctx, `
		SELECT id, account_id, email::text, status, attempts, last_error,
			next_run_at, last_run_at, created_at, updated_at, completed_at
		FROM mailbox_provision_jobs
		ORDER BY updated_at DESC
		LIMIT 300
	`)
	if err != nil {
		return AppState{}, false, fmt.Errorf("load provision jobs: %w", err)
	}
	defer provisionRows.Close()
	for provisionRows.Next() {
		var job ProvisionJob
		var nextRunAt sql.NullTime
		var lastRunAt sql.NullTime
		var createdAt time.Time
		var updatedAt time.Time
		var completedAt sql.NullTime
		if err := provisionRows.Scan(
			&job.ID,
			&job.AccountID,
			&job.Email,
			&job.Status,
			&job.Attempts,
			&job.LastError,
			&nextRunAt,
			&lastRunAt,
			&createdAt,
			&updatedAt,
			&completedAt,
		); err != nil {
			return AppState{}, false, fmt.Errorf("scan provision job: %w", err)
		}
		job.NextRunAt = nullTimeToISO(nextRunAt)
		job.LastRunAt = nullTimeToISO(lastRunAt)
		job.CreatedAt = timeToISO(createdAt)
		job.UpdatedAt = timeToISO(updatedAt)
		job.CompletedAt = nullTimeToISO(completedAt)
		state.Config.ProvisionJobs = append(state.Config.ProvisionJobs, job)
	}
	if err := provisionRows.Err(); err != nil {
		return AppState{}, false, fmt.Errorf("iterate provision jobs: %w", err)
	}

	sessionRows, err := p.pool.Query(ctx, `
		SELECT token_hash, account_id, COALESCE(session_id, ''), COALESCE(ip, ''),
			COALESCE(user_agent, ''), COALESCE(device_label, ''),
			created_at, COALESCE(last_seen_at, created_at), expires_at
		FROM auth_sessions
		WHERE expires_at > now()
	`)
	if err != nil {
		return AppState{}, false, fmt.Errorf("load sessions: %w", err)
	}
	defer sessionRows.Close()
	for sessionRows.Next() {
		var key string
		var session SessionRecord
		var createdAt time.Time
		var lastSeenAt time.Time
		var expiresAt time.Time
		if err := sessionRows.Scan(
			&key,
			&session.UserID,
			&session.ID,
			&session.IP,
			&session.UserAgent,
			&session.Device,
			&createdAt,
			&lastSeenAt,
			&expiresAt,
		); err != nil {
			return AppState{}, false, fmt.Errorf("scan session: %w", err)
		}
		session.CreatedAt = timeToISO(createdAt)
		session.LastSeenAt = timeToISO(lastSeenAt)
		session.ExpiresAt = timeToISO(expiresAt)
		state.Sessions[key] = normalizeSessionRecord(key, session)
	}
	if err := sessionRows.Err(); err != nil {
		return AppState{}, false, fmt.Errorf("iterate sessions: %w", err)
	}

	settingsRows, err := p.pool.Query(ctx, `
		SELECT account_id, default_sender_name, signature, auto_reply_enabled, auto_reply_message, updated_at
		FROM mail_settings
	`)
	if err != nil {
		return AppState{}, false, fmt.Errorf("load settings: %w", err)
	}
	defer settingsRows.Close()
	for settingsRows.Next() {
		var accountID string
		var settings MailSettings
		var updatedAt time.Time
		if err := settingsRows.Scan(&accountID, &settings.DefaultSenderName, &settings.Signature, &settings.AutoReplyEnabled, &settings.AutoReplyMessage, &updatedAt); err != nil {
			return AppState{}, false, fmt.Errorf("scan settings: %w", err)
		}
		settings.UpdatedAt = timeToISO(updatedAt)
		state.Settings[accountID] = settings
	}
	if err := settingsRows.Err(); err != nil {
		return AppState{}, false, fmt.Errorf("iterate settings: %w", err)
	}

	messageRows, err := p.pool.Query(ctx, `
		SELECT account_id, id, thread_id, folder, previous_folder, sender, sender_email::text, recipients,
			avatar, role, subject, snippet, time_label, date_time_label, sort_at, sent_at, received_at,
			is_unread, is_starred, has_attachment, tags, is_outgoing, content_html, attachments, source, delivery_status,
			accepted_at, provider_message_id, delivery_error
		FROM mail_messages
		ORDER BY sort_at DESC
	`)
	if err != nil {
		return AppState{}, false, fmt.Errorf("load messages: %w", err)
	}
	defer messageRows.Close()
	for messageRows.Next() {
		var accountID string
		var message MailMessage
		var recipientsRaw []byte
		var tagsRaw []byte
		var attachmentsRaw []byte
		var sortAt time.Time
		var sentAt sql.NullTime
		var receivedAt sql.NullTime
		var acceptedAt sql.NullTime
		if err := messageRows.Scan(
			&accountID,
			&message.ID,
			&message.ThreadID,
			&message.Folder,
			&message.PreviousFolder,
			&message.Sender,
			&message.SenderEmail,
			&recipientsRaw,
			&message.Avatar,
			&message.Role,
			&message.Subject,
			&message.Snippet,
			&message.Time,
			&message.DateTimeLabel,
			&sortAt,
			&sentAt,
			&receivedAt,
			&message.IsUnread,
			&message.IsStarred,
			&message.HasAttachment,
			&tagsRaw,
			&message.IsOutgoing,
			&message.Content,
			&attachmentsRaw,
			&message.Source,
			&message.DeliveryStatus,
			&acceptedAt,
			&message.ProviderMessageID,
			&message.DeliveryError,
		); err != nil {
			return AppState{}, false, fmt.Errorf("scan message: %w", err)
		}
		if err := json.Unmarshal(recipientsRaw, &message.Recipients); err != nil {
			return AppState{}, false, fmt.Errorf("decode message recipients: %w", err)
		}
		if err := json.Unmarshal(tagsRaw, &message.Tags); err != nil {
			return AppState{}, false, fmt.Errorf("decode message tags: %w", err)
		}
		if err := json.Unmarshal(attachmentsRaw, &message.Attachments); err != nil {
			return AppState{}, false, fmt.Errorf("decode message attachments: %w", err)
		}
		message.SortAt = timeToISO(sortAt)
		message.SentAt = nullTimeToISO(sentAt)
		message.ReceivedAt = nullTimeToISO(receivedAt)
		message.AcceptedAt = nullTimeToISO(acceptedAt)
		state.Messages[accountID] = append(state.Messages[accountID], message)
	}
	if err := messageRows.Err(); err != nil {
		return AppState{}, false, fmt.Errorf("iterate messages: %w", err)
	}

	return state, true, nil
}

func (p *PostgresStateStore) saveNormalized(ctx context.Context, state AppState) error {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin normalized save: %w", err)
	}
	defer tx.Rollback(ctx)

	mailboxRaw, err := json.Marshal(state.Config.Mailbox)
	if err != nil {
		return fmt.Errorf("encode mailbox config: %w", err)
	}
	authRaw, err := json.Marshal(state.Config.Auth)
	if err != nil {
		return fmt.Errorf("encode auth config: %w", err)
	}
	smsRaw, err := json.Marshal(state.Config.SMS)
	if err != nil {
		return fmt.Errorf("encode sms config: %w", err)
	}
	opsRaw, err := json.Marshal(state.Config.Ops)
	if err != nil {
		return fmt.Errorf("encode ops config: %w", err)
	}
	securityRaw, err := json.Marshal(state.Config.Security)
	if err != nil {
		return fmt.Errorf("encode security config: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO admin_mail_config (id, mailbox, auth, sms, ops, security, updated_at)
		VALUES (true, $1, $2, $3, $4, $5, $6)
		ON CONFLICT (id) DO UPDATE
		SET mailbox = EXCLUDED.mailbox, auth = EXCLUDED.auth, sms = EXCLUDED.sms, ops = EXCLUDED.ops, security = EXCLUDED.security, updated_at = EXCLUDED.updated_at
	`, mailboxRaw, authRaw, smsRaw, opsRaw, securityRaw, timeFromISOOrNow(state.Config.UpdatedAt)); err != nil {
		return fmt.Errorf("save admin config: %w", err)
	}

	for _, statement := range []string{
		`DELETE FROM auth_sessions`,
		`DELETE FROM mail_messages`,
		`DELETE FROM mail_settings`,
		`DELETE FROM mail_accounts`,
		`DELETE FROM mailbox_provision_jobs`,
		`DELETE FROM mailbox_invites`,
		`DELETE FROM sms_code_logs`,
		`DELETE FROM audit_logs`,
	} {
		if _, err := tx.Exec(ctx, statement); err != nil {
			return fmt.Errorf("clear normalized table: %w", err)
		}
	}

	accountIDs := map[string]bool{}
	for _, user := range state.Users {
		if strings.TrimSpace(user.ID) == "" {
			continue
		}
		accountIDs[user.ID] = true
		if _, err := tx.Exec(ctx, `
			INSERT INTO mail_accounts (
				id, phone, email, display_name, password_hash, source, registered_at,
				status, disabled_at, password_reset_at,
				mailbox_status, mailbox_provisioned_at, mailbox_external_id, mailbox_last_error,
				mailbox_username, mailbox_password_secret,
				updated_at
			)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, now())
		`,
			user.ID,
			user.Phone,
			user.Email,
			firstNonEmpty(user.DisplayName, "MyName"),
			user.PasswordHash,
			firstNonEmpty(user.Source, "invite"),
			timeFromISOOrNow(user.RegisteredAt),
			normalizeAccountStatus(user.Status),
			timeFromISOOrNil(user.DisabledAt),
			timeFromISOOrNil(user.LastPasswordResetAt),
			normalizeMailboxStatus(user.MailboxStatus),
			timeFromISOOrNil(user.MailboxProvisionedAt),
			user.MailboxExternalID,
			user.MailboxLastError,
			firstNonEmpty(user.MailboxUsername, user.Email),
			user.MailboxPasswordSecret,
		); err != nil {
			return fmt.Errorf("save account %s: %w", user.ID, err)
		}
	}

	for _, job := range state.Config.ProvisionJobs {
		job = normalizeProvisionJob(job)
		if strings.TrimSpace(job.ID) == "" {
			continue
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO mailbox_provision_jobs (
				id, account_id, email, status, attempts, last_error,
				next_run_at, last_run_at, created_at, updated_at, completed_at
			)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		`,
			job.ID,
			job.AccountID,
			job.Email,
			normalizeProvisionJobStatus(job.Status),
			job.Attempts,
			job.LastError,
			timeFromISOOrNil(job.NextRunAt),
			timeFromISOOrNil(job.LastRunAt),
			timeFromISOOrNow(job.CreatedAt),
			timeFromISOOrNow(job.UpdatedAt),
			timeFromISOOrNil(job.CompletedAt),
		); err != nil {
			return fmt.Errorf("save provision job %s: %w", job.ID, err)
		}
	}

	for _, invite := range state.Config.Invites {
		if strings.TrimSpace(invite.ID) == "" {
			continue
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO mailbox_invites (id, code, email, mailbox_local_part, phone, note, created_at, expires_at, used_at, url)
			VALUES ($1, $2, $3, $4, NULLIF($5, ''), $6, $7, $8, $9, $10)
		`,
			invite.ID,
			strings.ToUpper(strings.TrimSpace(invite.Code)),
			invite.Email,
			invite.MailboxLocalPart,
			invite.Phone,
			invite.Note,
			timeFromISOOrNow(invite.CreatedAt),
			timeFromISOOrNil(invite.ExpiresAt),
			timeFromISOOrNil(invite.UsedAt),
			invite.URL,
		); err != nil {
			return fmt.Errorf("save invite %s: %w", invite.ID, err)
		}
	}

	for _, log := range state.Config.SMSLogs {
		if strings.TrimSpace(log.ID) == "" {
			continue
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO sms_code_logs (id, phone, code, code_hash, code_masked, purpose, provider, status, created_at, expires_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		`,
			log.ID,
			log.Phone,
			log.Code,
			log.CodeHash,
			firstNonEmpty(log.CodeMasked, maskSMSCode(log.Code)),
			firstNonEmpty(log.Purpose, "login"),
			firstNonEmpty(log.Provider, "local"),
			firstNonEmpty(log.Status, "local_sent"),
			timeFromISOOrNow(log.CreatedAt),
			timeFromISOOrNow(log.ExpiresAt),
		); err != nil {
			return fmt.Errorf("save sms log %s: %w", log.ID, err)
		}
	}

	for _, log := range state.Config.AuditLogs {
		if strings.TrimSpace(log.ID) == "" {
			continue
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO audit_logs (id, actor, action, target, detail, ip, created_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7)
		`,
			log.ID,
			firstNonEmpty(log.Actor, "system"),
			log.Action,
			log.Target,
			log.Detail,
			log.IP,
			timeFromISOOrNow(log.CreatedAt),
		); err != nil {
			return fmt.Errorf("save audit log %s: %w", log.ID, err)
		}
	}

	for storedKey, session := range state.Sessions {
		if !accountIDs[session.UserID] || sessionExpired(session) {
			continue
		}
		session = normalizeSessionRecord(storedKey, session)
		tokenKey := storedKey
		if session.Token != "" {
			tokenKey = sessionStorageKey(session.Token)
		} else if !isSessionStorageKey(tokenKey) {
			tokenKey = sessionStorageKey(tokenKey)
		}
		if tokenKey == sessionStorageKey("") {
			continue
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO auth_sessions (
				token_hash, account_id, session_id, ip, user_agent, device_label,
				created_at, last_seen_at, expires_at
			)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		`,
			tokenKey,
			session.UserID,
			session.ID,
			session.IP,
			session.UserAgent,
			firstNonEmpty(session.Device, deviceLabelFromUserAgent(session.UserAgent)),
			timeFromISOOrNow(session.CreatedAt),
			timeFromISOOrNow(firstNonEmpty(session.LastSeenAt, session.CreatedAt)),
			timeFromISOOrNow(session.ExpiresAt),
		); err != nil {
			return fmt.Errorf("save session: %w", err)
		}
	}

	for accountID, settings := range state.Settings {
		if !accountIDs[accountID] {
			continue
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO mail_settings (account_id, default_sender_name, signature, auto_reply_enabled, auto_reply_message, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6)
		`,
			accountID,
			settings.DefaultSenderName,
			settings.Signature,
			settings.AutoReplyEnabled,
			settings.AutoReplyMessage,
			timeFromISOOrNow(settings.UpdatedAt),
		); err != nil {
			return fmt.Errorf("save settings %s: %w", accountID, err)
		}
	}

	for accountID, messages := range state.Messages {
		if !accountIDs[accountID] {
			continue
		}
		for _, message := range messages {
			recipientsRaw, err := json.Marshal(nonNilStrings(message.Recipients))
			if err != nil {
				return fmt.Errorf("encode message recipients: %w", err)
			}
			tagsRaw, err := json.Marshal(nonNilStrings(message.Tags))
			if err != nil {
				return fmt.Errorf("encode message tags: %w", err)
			}
			attachmentsRaw, err := json.Marshal(nonNilAnys(message.Attachments))
			if err != nil {
				return fmt.Errorf("encode message attachments: %w", err)
			}
			if _, err := tx.Exec(ctx, `
				INSERT INTO mail_messages (
					id, account_id, thread_id, folder, previous_folder, sender, sender_email, recipients,
					avatar, role, subject, snippet, time_label, date_time_label, sort_at, sent_at, received_at,
					is_unread, is_starred, has_attachment, tags, is_outgoing, content_html, attachments, source, delivery_status,
					accepted_at, provider_message_id, delivery_error
				)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22, $23, $24, $25, $26, $27, $28, $29)
			`,
				message.ID,
				accountID,
				message.ThreadID,
				firstNonEmpty(message.Folder, "inbox"),
				firstNonEmpty(message.PreviousFolder, "inbox"),
				message.Sender,
				message.SenderEmail,
				recipientsRaw,
				message.Avatar,
				message.Role,
				message.Subject,
				message.Snippet,
				message.Time,
				message.DateTimeLabel,
				timeFromISOOrNow(message.SortAt),
				timeFromISOOrNil(message.SentAt),
				timeFromISOOrNil(message.ReceivedAt),
				message.IsUnread,
				message.IsStarred,
				message.HasAttachment,
				tagsRaw,
				message.IsOutgoing,
				message.Content,
				attachmentsRaw,
				firstNonEmpty(message.Source, "mailbox"),
				firstNonEmpty(message.DeliveryStatus, "received"),
				timeFromISOOrNil(message.AcceptedAt),
				message.ProviderMessageID,
				message.DeliveryError,
			); err != nil {
				return fmt.Errorf("save message %s: %w", message.ID, err)
			}
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit normalized save: %w", err)
	}
	return nil
}

func (p *PostgresStateStore) ready(ctx context.Context) error {
	return p.pool.Ping(ctx)
}

func (p *PostgresStateStore) close() {
	if p != nil && p.pool != nil {
		p.pool.Close()
	}
}

func (s *Store) snapshot() AppState {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.normalizeLocked()
	return cloneState(s.state)
}

func (s *Store) mutate(fn func(*AppState) (any, error)) (any, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.normalizeLocked()
	result, err := fn(&s.state)
	if err != nil {
		return nil, err
	}
	s.normalizeLocked()
	if err := s.saveLocked(); err != nil {
		return nil, err
	}
	return result, nil
}

func (s *Store) normalizeLocked() {
	defaults := defaultAdminConfig()
	if strings.TrimSpace(s.state.Config.Mailbox.Domain) == "" {
		s.state.Config.Mailbox = defaults.Mailbox
	}
	if strings.TrimSpace(s.state.Config.Auth.OAuthProviderName) == "" {
		s.state.Config.Auth = defaults.Auth
	}
	if strings.TrimSpace(s.state.Config.SMS.Provider) == "" {
		s.state.Config.SMS = defaults.SMS
	}
	if s.state.Config.Ops.IntervalMinutes <= 0 {
		s.state.Config.Ops = defaults.Ops
	}
	s.state.Config.Mailbox.Domain = normalizeDomain(s.state.Config.Mailbox.Domain)
	s.state.Config.Mailbox.AllowedPrefixes = normalizeAllowedPrefixes(s.state.Config.Mailbox.AllowedPrefixes)
	s.state.Config.Mailbox.DefaultPrefix = resolveDefaultPrefix(s.state.Config.Mailbox)
	s.state.Config.Mailbox.DNS = normalizeDNSCheck(s.state.Config.Mailbox.Domain, s.state.Config.Mailbox.DNS)
	s.state.Config.Mailbox.Server = normalizeMailServerConfig(s.state.Config.Mailbox.Server)
	s.state.Config.Auth = normalizeAuthConfig(s.state.Config.Auth)
	s.state.Config.Ops = normalizeOpsConfig(s.state.Config.Ops)
	s.state.Config.Security = normalizeAdminSecurityConfig(s.state.Config.Security)
	if strings.TrimSpace(s.state.Config.SMS.AccessKeySecret) != "" {
		s.state.Config.SMS.AccessKeySecretSet = true
		s.state.Config.SMS.AccessKeySecretMasked = maskSecret(s.state.Config.SMS.AccessKeySecret)
	}
	if s.state.Config.SMS.CodeTTLMinutes <= 0 {
		s.state.Config.SMS.CodeTTLMinutes = defaults.SMS.CodeTTLMinutes
	}
	if s.state.Config.Invites == nil {
		s.state.Config.Invites = []InviteRecord{}
	}
	if s.state.Config.ProvisionJobs == nil {
		s.state.Config.ProvisionJobs = []ProvisionJob{}
	}
	for index := range s.state.Config.ProvisionJobs {
		s.state.Config.ProvisionJobs[index] = normalizeProvisionJob(s.state.Config.ProvisionJobs[index])
	}
	if s.state.Config.SMSLogs == nil {
		s.state.Config.SMSLogs = []SMSLogRecord{}
	}
	for index := range s.state.Config.SMSLogs {
		s.state.Config.SMSLogs[index] = normalizeSMSLogRecord(s.state.Config.SMSLogs[index])
	}
	if s.state.Config.AuditLogs == nil {
		s.state.Config.AuditLogs = []AuditLogRecord{}
	}
	if s.state.Config.RegisteredUsers == nil {
		s.state.Config.RegisteredUsers = []RegisteredUser{}
	}
	if s.state.Users == nil {
		s.state.Users = []UserRecord{}
	}
	for index := range s.state.Users {
		normalizeRegisteredUser(&s.state.Users[index].RegisteredUser)
	}
	for index := range s.state.Config.RegisteredUsers {
		normalizeRegisteredUser(&s.state.Config.RegisteredUsers[index])
	}
	if s.state.Sessions == nil {
		s.state.Sessions = map[string]SessionRecord{}
	}
	normalizedSessions := map[string]SessionRecord{}
	for key, session := range s.state.Sessions {
		if sessionExpired(session) {
			continue
		}
		normalizedKey := key
		if session.Token != "" {
			normalizedKey = sessionStorageKey(session.Token)
		} else if !isSessionStorageKey(normalizedKey) {
			normalizedKey = sessionStorageKey(normalizedKey)
		}
		if normalizedKey == sessionStorageKey("") {
			continue
		}
		normalizedSessions[normalizedKey] = normalizeSessionRecord(normalizedKey, session)
	}
	s.state.Sessions = normalizedSessions
	if s.state.Settings == nil {
		s.state.Settings = map[string]MailSettings{}
	}
	if s.state.Messages == nil {
		s.state.Messages = map[string][]MailMessage{}
	}
	s.state.Config.Usage = buildUsageSnapshot(s.state)
}

func (s *Store) saveLocked() error {
	if s.pg != nil {
		return s.pg.save(context.Background(), s.state)
	}
	if s.path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(s.state, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, raw, 0o600)
}

func (s *Store) ready(ctx context.Context) error {
	if s.pg == nil {
		return nil
	}
	return s.pg.ready(ctx)
}

func (s *Store) kind() string {
	if s != nil && s.pg != nil {
		return "postgres"
	}
	return "json"
}

func (s *Store) close() {
	if s.pg != nil {
		s.pg.close()
	}
}

func cloneState(state AppState) AppState {
	raw, _ := json.Marshal(state)
	var cloned AppState
	_ = json.Unmarshal(raw, &cloned)
	return cloned
}

func defaultAdminConfig() AdminConfig {
	return AdminConfig{
		Mailbox: MailboxConfig{
			Domain:              "yuexiang.com",
			PrefixPolicyEnabled: true,
			AllowedPrefixes:     []string{"user", "admin", "support"},
			DefaultPrefix:       "user",
			DNS:                 defaultDNSCheck("yuexiang.com"),
			Server:              defaultMailServerConfig(),
		},
		Auth: AuthConfig{
			OAuthEnabled:         true,
			OAuthProviderName:    "悦享账号",
			OAuthScopes:          []string{"openid", "profile", "email", "phone"},
			OAuthSubjectField:    "sub",
			OAuthPhoneField:      "phone",
			OAuthEmailField:      "email",
			OAuthNameField:       "name",
			PasswordLoginEnabled: true,
			PhoneLoginEnabled:    true,
			EmailLoginEnabled:    false,
			RegistrationEnabled:  true,
			InviteRequired:       true,
			LoginLandingMode:     "oauth",
		},
		SMS: SMSConfig{
			Provider:       "aliyun",
			AliyunEnabled:  false,
			CodeTTLMinutes: 5,
		},
		Security:        AdminSecurityConfig{Username: "admin"},
		Ops:             OpsConfig{AutoRunEnabled: false, IntervalMinutes: 5, LastRunStatus: "idle", LastRunMessage: "自动巡检未开启"},
		Usage:           UsageSnapshot{},
		Invites:         []InviteRecord{},
		ProvisionJobs:   []ProvisionJob{},
		SMSLogs:         []SMSLogRecord{},
		AuditLogs:       []AuditLogRecord{},
		RegisteredUsers: []RegisteredUser{},
	}
}

func redactAdminConfig(config AdminConfig) AdminConfig {
	config.Mailbox.Server = redactMailServerConfig(config.Mailbox.Server)
	config.Auth = redactAuthConfig(config.Auth)
	config.SMS = redactSMSConfig(config.SMS)
	config.Security = redactAdminSecurityConfig(config.Security)
	config.SMSLogs = redactSMSLogs(config.SMSLogs)
	config.Deployment = buildDeploymentStatus(config, storeKindFromEnv())
	return config
}

func publicAdminConfig(config AdminConfig) AdminConfig {
	config = redactAdminConfig(config)
	config.Mailbox.Server = MailServerConfig{}
	config.Ops = OpsConfig{}
	config.Usage = UsageSnapshot{}
	config.Security = AdminSecurityConfig{}
	config.Invites = []InviteRecord{}
	config.ProvisionJobs = []ProvisionJob{}
	config.SMSLogs = []SMSLogRecord{}
	config.AuditLogs = []AuditLogRecord{}
	config.RegisteredUsers = []RegisteredUser{}
	return config
}

func redactSMSLogs(logs []SMSLogRecord) []SMSLogRecord {
	if logs == nil {
		return []SMSLogRecord{}
	}
	redacted := make([]SMSLogRecord, 0, len(logs))
	for _, log := range logs {
		redacted = append(redacted, redactSMSLog(log))
	}
	return redacted
}

func redactSMSLog(log SMSLogRecord) SMSLogRecord {
	log = normalizeSMSLogRecord(log)
	log.CodeHash = ""
	if strings.TrimSpace(log.CodeMasked) == "" {
		log.CodeMasked = maskSMSCode(log.Code)
	}
	if !shouldExposeSMSDebugCode(log) {
		log.Code = ""
	}
	return log
}

func shouldExposeSMSDebugCode(log SMSLogRecord) bool {
	return strings.EqualFold(strings.TrimSpace(log.Provider), "local") && localSMSDebugAllowedFromEnv() && !envBool("HIDE_LOCAL_SMS_CODES", false)
}

func (app *App) localSMSDebugAllowed() bool {
	if app == nil || productionStrictEnabled() {
		return false
	}
	return app.allowLocalSMSDebug || envBool("ALLOW_LOCAL_SMS_DEBUG", false)
}

func localSMSDebugAllowedFromEnv() bool {
	return !productionStrictEnabled() && envBool("ALLOW_LOCAL_SMS_DEBUG", false)
}

func normalizeSMSLogRecord(log SMSLogRecord) SMSLogRecord {
	log.Phone = normalizePhone(log.Phone)
	log.Purpose = firstNonEmpty(strings.TrimSpace(log.Purpose), "login")
	provider := strings.ToLower(strings.TrimSpace(log.Provider))
	if provider == "" {
		provider = "local"
	}
	log.Provider = provider
	status := strings.ToLower(strings.TrimSpace(log.Status))
	if status == "" {
		if provider == "aliyun" {
			status = "sent"
		} else {
			status = "local_sent"
		}
	}
	log.Status = status
	log.Code = strings.TrimSpace(log.Code)
	log.CodeHash = strings.TrimSpace(log.CodeHash)
	if strings.TrimSpace(log.CodeMasked) == "" {
		log.CodeMasked = maskSMSCode(log.Code)
	}
	return log
}

func redactAuthConfig(config AuthConfig) AuthConfig {
	config = normalizeAuthConfig(config)
	if strings.TrimSpace(config.OAuthClientSecret) != "" {
		config.OAuthClientSecretSet = true
		config.OAuthClientSecretMasked = maskSecret(config.OAuthClientSecret)
	}
	config.OAuthClientSecret = ""
	return config
}

func redactSMSConfig(config SMSConfig) SMSConfig {
	if strings.TrimSpace(config.AccessKeySecret) != "" {
		config.AccessKeySecretSet = true
		config.AccessKeySecretMasked = maskSecret(config.AccessKeySecret)
	}
	config.AccessKeySecret = ""
	return config
}

func redactMailServerConfig(config MailServerConfig) MailServerConfig {
	config = normalizeMailServerConfig(config)
	if strings.TrimSpace(config.AdminToken) != "" {
		config.AdminTokenSet = true
		config.AdminTokenMasked = maskSecret(config.AdminToken)
	}
	config.AdminToken = ""
	if strings.TrimSpace(config.SMTPPassword) != "" {
		config.SMTPPasswordSet = true
		config.SMTPPasswordMasked = maskSecret(config.SMTPPassword)
	}
	config.SMTPPassword = ""
	if strings.TrimSpace(config.IMAPPassword) != "" {
		config.IMAPPasswordSet = true
		config.IMAPPasswordMasked = maskSecret(config.IMAPPassword)
	}
	config.IMAPPassword = ""
	return config
}

func redactAdminSecurityConfig(config AdminSecurityConfig) AdminSecurityConfig {
	config = normalizeAdminSecurityConfig(config)
	config.PasswordHash = ""
	config.APITokenHash = ""
	return config
}

func (app *App) health(_ *http.Request) (int, any) {
	return http.StatusOK, response{"ok": true, "service": "infinitemail-bff", "time": time.Now().UTC().Format(time.RFC3339)}
}

func (app *App) ready(_ *http.Request) (int, any) {
	if err := app.store.ready(context.Background()); err != nil {
		return http.StatusServiceUnavailable, response{"ready": false, "message": "postgres not ready"}
	}
	state := app.store.snapshot()
	deployment := buildDeploymentStatus(state.Config, app.store.kind())
	if productionStrictEnabled() && !deployment.Ready {
		return http.StatusServiceUnavailable, response{"ready": false, "deployment": deployment}
	}
	return http.StatusOK, response{"ready": deployment.Ready, "deployment": deployment}
}

func buildDeploymentStatus(config AdminConfig, storeKind string) DeploymentStatus {
	strict := productionStrictEnabled()
	config.Mailbox.Server = normalizeMailServerConfig(config.Mailbox.Server)
	config.Auth = normalizeAuthConfig(config.Auth)
	config.Security = normalizeAdminSecurityConfig(config.Security)
	if storeKind == "" {
		storeKind = storeKindFromEnv()
	}
	checks := []DeploymentCheck{}
	add := func(id string, label string, required bool, ok bool, message string) {
		status := "ok"
		if !ok && required {
			status = "blocking"
		} else if !ok {
			status = "warning"
		}
		checks = append(checks, DeploymentCheck{
			ID:       id,
			Label:    label,
			Status:   status,
			Required: required,
			Message:  message,
		})
	}

	storeRequired := true
	add("store", "PostgreSQL 持久化", storeRequired, storeKind == "postgres", mapBoolMessage(storeKind == "postgres", "已使用 PostgreSQL 业务表持久化", "当前是 JSON 文件存储，仅适合本地开发；生产请配置 PostgreSQL"))

	attachmentReady := attachmentStoreReady(env("ATTACHMENT_DIR", filepath.Join(".data", "attachments")))
	add("attachment_store", "附件文件存储", true, attachmentReady, mapBoolMessage(attachmentReady, "附件目录可写，邮件附件会保存为鉴权下载文件", "附件目录不可写，无法保证附件上传、下载和投递"))

	adminAuthReady := strings.TrimSpace(env("ADMIN_API_TOKEN", "")) != "" ||
		strings.TrimSpace(env("ADMIN_PASSWORD", "")) != "" ||
		strings.TrimSpace(env("ADMIN_PASSWORD_HASH", "")) != "" ||
		normalizeAdminSecurityConfig(config.Security).PasswordSet ||
		normalizeAdminSecurityConfig(config.Security).APITokenSet
	add("admin_auth", "后台访问保护", true, adminAuthReady, mapBoolMessage(adminAuthReady, "后台已配置登录密码或管理令牌", "生产环境必须设置后台管理员密码、ADMIN_PASSWORD_HASH 或 ADMIN_API_TOKEN"))

	credentialKeyReady := strings.TrimSpace(env("MAILBOX_CREDENTIAL_KEY", "")) != ""
	add("mailbox_credential_key", "邮箱凭据加密密钥", true, credentialKeyReady, mapBoolMessage(credentialKeyReady, "邮箱开通凭据将使用服务端密钥加密保存", "生产环境必须配置 MAILBOX_CREDENTIAL_KEY，禁止明文保存邮箱密码"))

	dnsVerified := config.Mailbox.DNS.Status == "verified"
	add("dns", "域名 DNS 验证", true, dnsVerified, mapBoolMessage(dnsVerified, "MX/SPF/DKIM/DMARC 检查已通过", "请在后台执行 DNS 验证并修正未通过记录"))

	controlMissing := mailServerControlMissing(config.Mailbox.Server)
	add("mail_control", "邮箱开通与生命周期", true, len(controlMissing) == 0, missingMessage(controlMissing, "开通、禁用、启用、重置密码接口已配置"))

	dataPlaneRequired := true
	dataPlaneMissing := mailServerDataPlaneMissing(config.Mailbox.Server)
	add("mail_data_plane", "邮件收发数据面", dataPlaneRequired, len(dataPlaneMissing) == 0, missingMessage(dataPlaneMissing, "HTTP 邮件数据面或 SMTP/IMAP 协议已配置"))

	webhookRequired := requireMailWebhook()
	webhookReady := strings.TrimSpace(env("MAIL_WEBHOOK_TOKEN", "")) != ""
	add("mail_webhook", "收信与投递 Webhook", webhookRequired, webhookReady, mapBoolMessage(webhookReady, "收信和投递回调已配置鉴权令牌", "如邮件底座使用 Webhook 写入 BFF，请配置 MAIL_WEBHOOK_TOKEN"))

	smsRequired := config.Auth.PhoneLoginEnabled || config.Auth.RegistrationEnabled || requireRealSMSProvider()
	smsReady := smsConfigComplete(config.SMS)
	smsMessage := "阿里云短信 AccessKey、签名、模板已配置"
	if !smsReady {
		smsMessage = "手机号登录注册开启时，生产环境必须启用并完整配置阿里云短信"
	}
	add("sms", "短信验证码", smsRequired, smsReady, smsMessage)

	oauthRequired := config.Auth.OAuthEnabled || requireRealOAuthProvider()
	oauthReady := oauthConfigComplete(config.Auth)
	oauthMessage := "OAuth 授权、Token、用户信息接口已配置"
	if !oauthReady {
		oauthMessage = "OAuth 开启时，生产环境必须配置真实 OAuth/OIDC Provider"
	}
	add("oauth", "OAuth 统一登录", oauthRequired, oauthReady || (!oauthRequired && !config.Auth.OAuthEnabled), oauthMessage)

	blocking := 0
	for _, check := range checks {
		if check.Status == "blocking" {
			blocking += 1
		}
	}
	status := "ready"
	if blocking > 0 {
		status = "blocked"
	} else if !strict {
		status = "ready"
	}
	return DeploymentStatus{
		Strict:        strict,
		Ready:         blocking == 0,
		Status:        status,
		Store:         storeKind,
		UpdatedAt:     nowISO(),
		BlockingCount: blocking,
		Checks:        checks,
	}
}

func mapBoolMessage(ok bool, success string, failure string) string {
	if ok {
		return success
	}
	return failure
}

func missingMessage(missing []string, success string) string {
	if len(missing) == 0 {
		return success
	}
	return "缺少：" + strings.Join(missing, "、")
}

func attachmentStoreReady(dir string) bool {
	dir = strings.TrimSpace(dir)
	if dir == "" {
		dir = filepath.Join(".data", "attachments")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return false
	}
	testPath := filepath.Join(dir, ".write-test")
	if err := os.WriteFile(testPath, []byte("ok"), 0o600); err != nil {
		return false
	}
	_ = os.Remove(testPath)
	return true
}

func smsConfigComplete(config SMSConfig) bool {
	return config.AliyunEnabled &&
		strings.TrimSpace(config.AccessKeyID) != "" &&
		strings.TrimSpace(config.AccessKeySecret) != "" &&
		strings.TrimSpace(config.SignName) != "" &&
		strings.TrimSpace(config.TemplateCode) != ""
}

func oauthConfigComplete(config AuthConfig) bool {
	config = normalizeAuthConfig(config)
	return strings.TrimSpace(config.OAuthClientID) != "" &&
		strings.TrimSpace(config.OAuthClientSecret) != "" &&
		strings.TrimSpace(config.OAuthAuthorizeURL) != "" &&
		strings.TrimSpace(config.OAuthTokenURL) != "" &&
		strings.TrimSpace(config.OAuthUserInfoURL) != "" &&
		strings.TrimSpace(config.OAuthRedirectURL) != ""
}

func mailServerControlMissing(config MailServerConfig) []string {
	config = normalizeMailServerConfig(config)
	missing := []string{}
	if !config.Enabled {
		missing = append(missing, "启用邮件服务")
	}
	if strings.TrimSpace(config.BaseURL) == "" && !smtpOutboundReady(config) {
		missing = append(missing, "服务地址")
	}
	if strings.TrimSpace(config.ProvisionPath) == "" && !stalwartJMAPReady(config) {
		missing = append(missing, "开通接口路径")
	}
	if strings.TrimSpace(config.LifecyclePath) == "" && !stalwartJMAPReady(config) {
		missing = append(missing, "账号生命周期接口")
	}
	return missing
}

func mailServerDataPlaneMissing(config MailServerConfig) []string {
	config = normalizeMailServerConfig(config)
	missing := []string{}
	imapReady := imapInboundReady(config)
	if !config.Enabled && !imapReady && !smtpOutboundReady(config) {
		missing = append(missing, "启用邮件服务")
	}
	if strings.TrimSpace(config.BaseURL) == "" && !imapReady {
		missing = append(missing, "服务地址或 IMAP 收件")
	}
	required := []struct {
		value string
		label string
	}{
		{config.MessageListPath, "收件列表接口"},
		{config.MessageDetailPath, "邮件详情接口"},
		{config.DraftPath, "草稿接口"},
		{config.MessageStarPath, "星标接口"},
		{config.MessageMovePath, "移动接口"},
		{config.MessageReadPath, "已读接口"},
	}
	for _, item := range required {
		if strings.TrimSpace(item.value) == "" && !imapReady {
			missing = append(missing, item.label)
		}
	}
	if strings.TrimSpace(config.MessageSendPath) == "" && !smtpOutboundReady(config) {
		missing = append(missing, "发信接口或 SMTP 发信")
	}
	return missing
}

func (app *App) getAdminAuthSession(r *http.Request) (int, any) {
	security := app.storedAdminSecurity()
	username := app.effectiveAdminUsername(security)
	authRequired := app.adminAuthRequired()
	if !authRequired {
		return http.StatusOK, response{"authRequired": false, "isAuthenticated": true, "username": username}
	}
	if app.adminAPIToken != "" && constantTimeStringEqual(adminTokenFromRequest(r), app.adminAPIToken) {
		return http.StatusOK, response{"authRequired": true, "isAuthenticated": true, "username": username}
	}
	if verifyAdminAPITokenHash(security.APITokenHash, adminTokenFromRequest(r)) {
		return http.StatusOK, response{"authRequired": true, "isAuthenticated": true, "username": username}
	}
	session, ok := app.lookupAdminSession(adminTokenFromRequest(r))
	if !ok || adminSessionExpired(session) {
		return http.StatusOK, response{"authRequired": true, "isAuthenticated": false, "username": username}
	}
	return http.StatusOK, response{
		"authRequired":    true,
		"isAuthenticated": true,
		"username":        session.Username,
		"expiresAt":       session.ExpiresAt,
	}
}

func (app *App) loginAdmin(r *http.Request) (int, any) {
	var payload struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := decodeJSON(r, &payload); err != nil {
		return badRequest("invalid json body")
	}
	security := app.storedAdminSecurity()
	defaultUsername := app.effectiveAdminUsername(security)
	if !app.adminAuthRequired() {
		return http.StatusOK, response{"authRequired": false, "isAuthenticated": true, "username": defaultUsername}
	}
	username := strings.TrimSpace(payload.Username)
	if username == "" {
		username = defaultUsername
	}
	if !app.verifyAdminCredentials(username, payload.Password, security) {
		app.appendAdminAuthAudit(r, "admin.auth.login_failed", username, "管理员登录失败")
		return http.StatusUnauthorized, response{"message": "管理员账号或密码错误"}
	}
	session := app.createAdminSession(username)
	app.appendAdminAuthAudit(r, "admin.auth.login", username, "管理员登录成功")
	return http.StatusOK, response{
		"authRequired":    true,
		"isAuthenticated": true,
		"username":        session.Username,
		"token":           session.Token,
		"expiresAt":       session.ExpiresAt,
	}
}

func (app *App) logoutAdmin(r *http.Request) (int, any) {
	token := adminTokenFromRequest(r)
	session, _ := app.lookupAdminSession(token)
	app.deleteAdminSession(token)
	app.appendAdminAuthAudit(r, "admin.auth.logout", firstNonEmpty(session.Username, app.adminUsername), "管理员退出后台")
	return http.StatusOK, response{"success": true}
}

func (app *App) appendAdminAuthAudit(r *http.Request, action string, target string, detail string) {
	_, _ = app.store.mutate(func(state *AppState) (any, error) {
		appendAuditLog(state, r, "admin", action, target, detail)
		return nil, nil
	})
}

func (app *App) getAdminConfig(_ *http.Request) (int, any) {
	state := app.store.snapshot()
	return http.StatusOK, redactAdminConfig(state.Config)
}

func (app *App) updateAdminConfig(r *http.Request) (int, any) {
	var patch adminConfigPatch
	if err := decodeJSON(r, &patch); err != nil {
		return badRequest("invalid json body")
	}
	result, err := app.store.mutate(func(state *AppState) (any, error) {
		if patch.Mailbox != nil {
			applyMailboxPatch(&state.Config.Mailbox, *patch.Mailbox)
		}
		if patch.Auth != nil {
			applyAuthPatch(&state.Config.Auth, *patch.Auth)
		}
		if patch.SMS != nil {
			applySMSPatch(&state.Config.SMS, *patch.SMS)
		}
		if patch.Ops != nil {
			applyOpsPatch(&state.Config.Ops, *patch.Ops)
		}
		if patch.Security != nil {
			if err := applyAdminSecurityPatch(&state.Config.Security, *patch.Security); err != nil {
				return nil, err
			}
		}
		state.Config.UpdatedAt = nowISO()
		appendAuditLog(state, r, "admin", "admin.config.update", "mail_config", "后台更新公司邮箱配置")
		return redactAdminConfig(state.Config), nil
	})
	if err != nil {
		return domainError(err)
	}
	return http.StatusOK, result
}

func (app *App) verifyMailboxDomain(r *http.Request) (int, any) {
	result, err := app.store.mutate(func(state *AppState) (any, error) {
		domain := normalizeDomain(state.Config.Mailbox.Domain)
		check := verifyDomainDNS(r.Context(), domain)
		state.Config.Mailbox.DNS = check
		state.Config.UpdatedAt = check.CheckedAt
		appendAuditLog(state, r, "admin", "admin.domain.verify", domain, check.Status)
		return redactAdminConfig(state.Config), nil
	})
	if err != nil {
		return domainError(err)
	}
	return http.StatusOK, result
}

func (app *App) testMailServer(r *http.Request) (int, any) {
	result, err := app.store.mutate(func(state *AppState) (any, error) {
		check := testMailServerConnection(r.Context(), state.Config.Mailbox.Server)
		state.Config.Mailbox.Server.Status = check.Status
		state.Config.Mailbox.Server.LastCheckedAt = check.LastCheckedAt
		state.Config.Mailbox.Server.LastError = check.LastError
		state.Config.UpdatedAt = check.LastCheckedAt
		appendAuditLog(state, r, "admin", "admin.mail_server.test", state.Config.Mailbox.Server.Provider, check.Status)
		return redactAdminConfig(state.Config), nil
	})
	if err != nil {
		return domainError(err)
	}
	return http.StatusOK, result
}

func (app *App) listInvites(_ *http.Request) (int, any) {
	state := app.store.snapshot()
	return http.StatusOK, response{"items": state.Config.Invites}
}

func (app *App) createInvite(r *http.Request) (int, any) {
	var payload struct {
		Prefix        string `json:"prefix"`
		EmailPrefix   string `json:"emailPrefix"`
		Phone         string `json:"phone"`
		Note          string `json:"note"`
		ExpiresInDays int    `json:"expiresInDays"`
	}
	if err := decodeJSON(r, &payload); err != nil {
		return badRequest("invalid json body")
	}
	result, err := app.store.mutate(func(state *AppState) (any, error) {
		localPart, err := buildMailboxLocalPart(state.Config.Mailbox, payload.EmailPrefix, payload.Prefix)
		if err != nil {
			return nil, err
		}
		email := localPart + "@" + state.Config.Mailbox.Domain
		if emailExists(state.Users, email) || activeInviteEmailExists(state.Config.Invites, email) {
			return nil, errors.New("这个邮箱名已经被占用")
		}
		phone := normalizePhone(payload.Phone)
		if phone != "" && !phoneRe.MatchString(phone) {
			return nil, errors.New("请输入 11 位手机号")
		}
		code := "INV-" + randomBase32(6)
		createdAt := nowISO()
		expiresAt := ""
		if payload.ExpiresInDays > 0 {
			expiresAt = time.Now().Add(time.Duration(payload.ExpiresInDays) * 24 * time.Hour).Format(time.RFC3339)
		}
		invite := InviteRecord{
			ID:               nextID("invite"),
			Code:             code,
			Email:            email,
			MailboxLocalPart: localPart,
			Phone:            phone,
			Note:             strings.TrimSpace(payload.Note),
			CreatedAt:        createdAt,
			ExpiresAt:        expiresAt,
			URL:              app.userAppOrigin + "/?invite=" + code,
		}
		state.Config.Invites = append([]InviteRecord{invite}, state.Config.Invites...)
		if len(state.Config.Invites) > 100 {
			state.Config.Invites = state.Config.Invites[:100]
		}
		appendAuditLog(state, r, "admin", "admin.invite.create", invite.Email, invite.Note)
		state.Config.UpdatedAt = createdAt
		return invite, nil
	})
	if err != nil {
		return domainError(err)
	}
	return http.StatusOK, result
}

func (app *App) listAccounts(_ *http.Request) (int, any) {
	state := app.store.snapshot()
	return http.StatusOK, response{"items": state.Config.RegisteredUsers}
}

func (app *App) disableAccount(r *http.Request) (int, any) {
	accountID := strings.TrimSpace(r.PathValue("accountID"))
	if accountID == "" {
		return badRequest("账号不存在")
	}
	result, err := app.store.mutate(func(state *AppState) (any, error) {
		index, ok := findUserIndexByID(state.Users, accountID)
		if !ok {
			return nil, errors.New("账号不存在")
		}
		user := &state.Users[index]
		now := nowISO()
		lifecycle, err := syncMailboxLifecycle(r.Context(), &state.Config, *user, "disable", "", "lifecycle-disable-"+user.ID+"-"+now)
		if err != nil {
			return nil, err
		}
		user.Status = "disabled"
		user.DisabledAt = now
		user.MailboxExternalID = firstNonEmpty(lifecycle.ExternalID, user.MailboxExternalID)
		updateRegisteredUser(state.Config.RegisteredUsers, user.RegisteredUser)
		deleteSessionsForUser(state.Sessions, user.ID)
		appendAuditLog(state, r, "admin", "admin.account.disable", user.Email, "后台禁用账号、同步邮件服务并清理会话")
		state.Config.UpdatedAt = now
		return response{"account": user.RegisteredUser}, nil
	})
	if err != nil {
		return domainError(err)
	}
	return http.StatusOK, result
}

func (app *App) enableAccount(r *http.Request) (int, any) {
	accountID := strings.TrimSpace(r.PathValue("accountID"))
	if accountID == "" {
		return badRequest("账号不存在")
	}
	result, err := app.store.mutate(func(state *AppState) (any, error) {
		index, ok := findUserIndexByID(state.Users, accountID)
		if !ok {
			return nil, errors.New("账号不存在")
		}
		user := &state.Users[index]
		now := nowISO()
		lifecycle, err := syncMailboxLifecycle(r.Context(), &state.Config, *user, "enable", "", "lifecycle-enable-"+user.ID+"-"+now)
		if err != nil {
			return nil, err
		}
		user.Status = "active"
		user.DisabledAt = ""
		user.MailboxExternalID = firstNonEmpty(lifecycle.ExternalID, user.MailboxExternalID)
		updateRegisteredUser(state.Config.RegisteredUsers, user.RegisteredUser)
		appendAuditLog(state, r, "admin", "admin.account.enable", user.Email, "后台启用账号并同步邮件服务")
		state.Config.UpdatedAt = now
		return response{"account": user.RegisteredUser}, nil
	})
	if err != nil {
		return domainError(err)
	}
	return http.StatusOK, result
}

func (app *App) resetAccountPassword(r *http.Request) (int, any) {
	accountID := strings.TrimSpace(r.PathValue("accountID"))
	if accountID == "" {
		return badRequest("账号不存在")
	}
	result, err := app.store.mutate(func(state *AppState) (any, error) {
		index, ok := findUserIndexByID(state.Users, accountID)
		if !ok {
			return nil, errors.New("账号不存在")
		}
		user := &state.Users[index]
		temporaryPassword := "Tmp-" + randomBase32(8) + randomDigits(2)
		passwordHash, err := hashPassword(temporaryPassword)
		if err != nil {
			return nil, err
		}
		now := nowISO()
		lifecycle, err := syncMailboxLifecycle(r.Context(), &state.Config, *user, "reset_password", temporaryPassword, "lifecycle-reset-password-"+user.ID+"-"+now)
		if err != nil {
			return nil, err
		}
		user.PasswordHash = passwordHash
		user.LastPasswordResetAt = now
		user.MailboxExternalID = firstNonEmpty(lifecycle.ExternalID, user.MailboxExternalID)
		if err := rememberMailboxCredential(user, MailboxProvisionResult{MailboxUsername: user.Email, MailboxPassword: temporaryPassword}); err != nil {
			return nil, err
		}
		updateRegisteredUser(state.Config.RegisteredUsers, user.RegisteredUser)
		deleteSessionsForUser(state.Sessions, user.ID)
		appendAuditLog(state, r, "admin", "admin.account.reset_password", user.Email, "后台重置临时密码、同步邮件服务并清理会话")
		state.Config.UpdatedAt = now
		return response{"account": user.RegisteredUser, "temporaryPassword": temporaryPassword}, nil
	})
	if err != nil {
		return domainError(err)
	}
	return http.StatusOK, result
}

func (app *App) retryMailboxProvision(r *http.Request) (int, any) {
	accountID := strings.TrimSpace(r.PathValue("accountID"))
	if accountID == "" {
		return badRequest("账号不存在")
	}
	result, err := app.store.mutate(func(state *AppState) (any, error) {
		index, ok := findUserIndexByID(state.Users, accountID)
		if !ok {
			return nil, errors.New("账号不存在")
		}
		user := &state.Users[index]
		now := time.Now()
		nowValue := now.Format(time.RFC3339)
		applyMailboxProvisionState(user, state.Config.Mailbox.Server)
		job := queueProvisionJob(state, *user, nowValue)
		jobIndex := -1
		for index := range state.Config.ProvisionJobs {
			if state.Config.ProvisionJobs[index].ID == job.ID {
				jobIndex = index
				break
			}
		}
		if jobIndex < 0 {
			return nil, errors.New("邮箱开通任务不存在")
		}
		jobPtr := &state.Config.ProvisionJobs[jobIndex]
		server := normalizeMailServerConfig(state.Config.Mailbox.Server)
		if !server.Enabled || server.BaseURL == "" {
			jobPtr.Status = "blocked"
			jobPtr.LastError = "邮件服务未配置"
			jobPtr.UpdatedAt = nowValue
			user.MailboxStatus = "pending_config"
			user.MailboxLastError = jobPtr.LastError
			user.MailboxProvisionedAt = ""
			updateRegisteredUser(state.Config.RegisteredUsers, user.RegisteredUser)
			appendAuditLog(state, r, "admin", "admin.account.provision_retry.blocked", user.Email, jobPtr.LastError)
			state.Config.UpdatedAt = nowValue
			return response{"account": user.RegisteredUser, "job": *jobPtr, "message": jobPtr.LastError}, nil
		}
		if server.Status != "online" {
			jobPtr.Status = "failed"
			jobPtr.LastError = firstNonEmpty(server.LastError, "邮件服务未连通")
			jobPtr.UpdatedAt = nowValue
			user.MailboxStatus = "failed"
			user.MailboxLastError = jobPtr.LastError
			user.MailboxProvisionedAt = ""
			updateRegisteredUser(state.Config.RegisteredUsers, user.RegisteredUser)
			appendAuditLog(state, r, "admin", "admin.account.provision_retry.failed", user.Email, jobPtr.LastError)
			state.Config.UpdatedAt = nowValue
			return response{"account": user.RegisteredUser, "job": *jobPtr, "message": jobPtr.LastError}, nil
		}
		jobPtr.Status = "running"
		jobPtr.Attempts += 1
		jobPtr.LastRunAt = nowValue
		jobPtr.UpdatedAt = nowValue
		provisioned, err := provisionMailboxAccount(r.Context(), state.Config, *user, *jobPtr)
		state.Config.Mailbox.Server.LastProvisionCheckAt = nowValue
		if err != nil {
			jobPtr.Status = "failed"
			jobPtr.LastError = err.Error()
			jobPtr.NextRunAt = now.Add(provisionRetryDelay(jobPtr.Attempts)).Format(time.RFC3339)
			jobPtr.UpdatedAt = nowValue
			user.MailboxStatus = "failed"
			user.MailboxLastError = jobPtr.LastError
			user.MailboxProvisionedAt = ""
			updateRegisteredUser(state.Config.RegisteredUsers, user.RegisteredUser)
			appendAuditLog(state, r, "admin", "admin.account.provision_retry.failed", user.Email, jobPtr.LastError)
			state.Config.UpdatedAt = nowValue
			return response{"account": user.RegisteredUser, "job": *jobPtr, "message": jobPtr.LastError}, nil
		}
		jobPtr.Status = "succeeded"
		jobPtr.LastError = ""
		jobPtr.NextRunAt = ""
		jobPtr.CompletedAt = nowValue
		jobPtr.UpdatedAt = nowValue
		user.MailboxStatus = "provisioned"
		user.MailboxProvisionedAt = nowValue
		user.MailboxExternalID = firstNonEmpty(provisioned.ExternalID, user.MailboxExternalID, "remote_"+user.ID)
		user.MailboxLastError = ""
		if err := rememberMailboxCredential(user, provisioned); err != nil {
			return nil, err
		}
		updateRegisteredUser(state.Config.RegisteredUsers, user.RegisteredUser)
		appendAuditLog(state, r, "admin", "admin.account.provision_retry.succeeded", user.Email, firstNonEmpty(provisioned.Status, "provisioned"))
		state.Config.UpdatedAt = nowValue
		return response{"account": user.RegisteredUser, "job": *jobPtr, "provision": provisioned, "message": "邮箱已真实开通"}, nil
	})
	if err != nil {
		return domainError(err)
	}
	if payload, ok := result.(response); ok {
		if job, ok := payload["job"].(ProvisionJob); ok && normalizeProvisionJobStatus(job.Status) != "succeeded" {
			return http.StatusBadRequest, payload
		}
	}
	return http.StatusOK, result
}

func (app *App) runOperationalTasks(r *http.Request) (int, any) {
	result, err := app.store.mutate(func(state *AppState) (any, error) {
		summary := runOperationalTasksState(r.Context(), state, time.Now())
		applyOperationalRunMetadata(&state.Config, summary)
		state.Config.UpdatedAt = summary.CheckedAt
		appendAuditLog(state, r, "admin", "admin.ops.run", "ops", summaryMessage(summary))
		return response{"summary": summary, "config": redactAdminConfig(state.Config)}, nil
	})
	if err != nil {
		return domainError(err)
	}
	return http.StatusOK, result
}

func (app *App) listSMSLogs(_ *http.Request) (int, any) {
	state := app.store.snapshot()
	return http.StatusOK, response{"items": redactSMSLogs(state.Config.SMSLogs)}
}

func (app *App) listAuditLogs(_ *http.Request) (int, any) {
	state := app.store.snapshot()
	return http.StatusOK, response{"items": state.Config.AuditLogs}
}

func (app *App) sendSMSCode(r *http.Request) (int, any) {
	var payload struct {
		Phone   string `json:"phone"`
		Purpose string `json:"purpose"`
	}
	if err := decodeJSON(r, &payload); err != nil {
		return badRequest("invalid json body")
	}
	phone := normalizePhone(payload.Phone)
	if !phoneRe.MatchString(phone) {
		return badRequest("请输入 11 位手机号")
	}
	purpose := strings.TrimSpace(payload.Purpose)
	if purpose == "" {
		purpose = "login"
	}
	result, err := app.store.mutate(func(state *AppState) (any, error) {
		realSMSReady := smsConfigComplete(state.Config.SMS)
		if !realSMSReady && !app.localSMSDebugAllowed() {
			return nil, errors.New("短信验证码未接入真实阿里云配置；本地调试需显式设置 ALLOW_LOCAL_SMS_DEBUG=true")
		}
		if err := ensureSMSCanSend(state.Config.SMSLogs, phone, purpose, time.Minute); err != nil {
			return nil, err
		}
		provider := "local"
		status := "local_sent"
		if realSMSReady {
			provider = "aliyun"
			status = "sent"
		}
		createdAt := nowISO()
		code := randomDigits(6)
		codeHash, err := hashSMSCode(phone, purpose, code)
		if err != nil {
			return nil, err
		}
		log := SMSLogRecord{
			ID:         nextID("sms"),
			Phone:      phone,
			Code:       code,
			CodeHash:   codeHash,
			CodeMasked: maskSMSCode(code),
			Purpose:    purpose,
			Provider:   provider,
			Status:     status,
			CreatedAt:  createdAt,
			ExpiresAt:  time.Now().Add(time.Duration(state.Config.SMS.CodeTTLMinutes) * time.Minute).Format(time.RFC3339),
		}
		if realSMSReady {
			if err := sendAliyunSMS(r.Context(), state.Config.SMS, phone, code); err != nil {
				appendAuditLog(state, r, "user", "auth.sms.send_failed", phone, purpose+" · aliyun · "+err.Error())
				return nil, err
			}
			log.Code = ""
		}
		state.Config.SMSLogs = append([]SMSLogRecord{log}, state.Config.SMSLogs...)
		if len(state.Config.SMSLogs) > 200 {
			state.Config.SMSLogs = state.Config.SMSLogs[:200]
		}
		appendAuditLog(state, r, "user", "auth.sms.send", phone, purpose+" · "+provider+" · "+status)
		state.Config.UpdatedAt = createdAt
		redacted := redactSMSLog(log)
		if provider == "local" && app.localSMSDebugAllowed() && !envBool("HIDE_LOCAL_SMS_CODES", false) {
			redacted.Code = log.Code
		}
		return redacted, nil
	})
	if err != nil {
		return domainError(err)
	}
	return http.StatusOK, result
}

func sendAliyunSMS(ctx context.Context, config SMSConfig, phone string, code string) error {
	if strings.TrimSpace(config.AccessKeyID) == "" || strings.TrimSpace(config.AccessKeySecret) == "" {
		return errors.New("阿里云短信 AccessKey 未完整配置")
	}
	if strings.TrimSpace(config.SignName) == "" || strings.TrimSpace(config.TemplateCode) == "" {
		return errors.New("阿里云短信签名或模板未配置")
	}

	params := map[string]string{
		"AccessKeyId":      strings.TrimSpace(config.AccessKeyID),
		"Action":           "SendSms",
		"Format":           "JSON",
		"PhoneNumbers":     phone,
		"RegionId":         "cn-hangzhou",
		"SignName":         strings.TrimSpace(config.SignName),
		"SignatureMethod":  "HMAC-SHA1",
		"SignatureNonce":   randomHex(16),
		"SignatureVersion": "1.0",
		"TemplateCode":     strings.TrimSpace(config.TemplateCode),
		"TemplateParam":    fmt.Sprintf(`{"code":"%s"}`, code),
		"Timestamp":        time.Now().UTC().Format("2006-01-02T15:04:05Z"),
		"Version":          "2017-05-25",
	}

	keys := make([]string, 0, len(params))
	for key := range params {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	encodedPairs := make([]string, 0, len(keys))
	for _, key := range keys {
		encodedPairs = append(encodedPairs, aliyunPercentEncode(key)+"="+aliyunPercentEncode(params[key]))
	}
	canonicalQuery := strings.Join(encodedPairs, "&")
	stringToSign := "GET&%2F&" + aliyunPercentEncode(canonicalQuery)
	mac := hmac.New(sha1.New, []byte(strings.TrimSpace(config.AccessKeySecret)+"&"))
	_, _ = mac.Write([]byte(stringToSign))
	signature := base64.StdEncoding.EncodeToString(mac.Sum(nil))

	endpoint := "https://dysmsapi.aliyuncs.com/?" + canonicalQuery + "&Signature=" + aliyunPercentEncode(signature)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return fmt.Errorf("创建阿里云短信请求失败: %w", err)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("阿里云短信请求失败: %w", err)
	}
	defer res.Body.Close()

	raw, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	var payload struct {
		Code    string `json:"Code"`
		Message string `json:"Message"`
	}
	_ = json.Unmarshal(raw, &payload)
	if res.StatusCode < 200 || res.StatusCode >= 300 || !strings.EqualFold(payload.Code, "OK") {
		message := strings.TrimSpace(payload.Message)
		if message == "" {
			message = strings.TrimSpace(string(raw))
		}
		if message == "" {
			message = http.StatusText(res.StatusCode)
		}
		return fmt.Errorf("阿里云短信发送失败: %s", message)
	}
	return nil
}

func aliyunPercentEncode(value string) string {
	encoded := url.QueryEscape(value)
	encoded = strings.ReplaceAll(encoded, "+", "%20")
	encoded = strings.ReplaceAll(encoded, "*", "%2A")
	encoded = strings.ReplaceAll(encoded, "%7E", "~")
	return encoded
}

func (app *App) registerWithPassword(r *http.Request) (int, any) {
	var payload struct {
		LoginType   string `json:"loginType"`
		Identifier  string `json:"identifier"`
		Phone       string `json:"phone"`
		Email       string `json:"email"`
		SMSCode     string `json:"smsCode"`
		Password    string `json:"password"`
		DisplayName string `json:"displayName"`
		EmailPrefix string `json:"emailPrefix"`
		Prefix      string `json:"prefix"`
		InviteCode  string `json:"inviteCode"`
	}
	if err := decodeJSON(r, &payload); err != nil {
		return badRequest("invalid json body")
	}
	if len(strings.TrimSpace(payload.Password)) < 6 {
		return badRequest("密码至少 6 位")
	}

	result, err := app.store.mutate(func(state *AppState) (any, error) {
		state.Config.Auth = normalizeAuthConfig(state.Config.Auth)
		if !state.Config.Auth.RegistrationEnabled {
			return nil, errors.New("后台已关闭普通注册")
		}
		identifier := firstNonEmpty(payload.Identifier, payload.Email, payload.Phone)
		loginMode := normalizeLoginMode(payload.LoginType, identifier, state.Config.Auth)
		phone := normalizePhone(firstNonEmpty(payload.Phone, payload.Identifier))
		if loginMode == "phone" {
			if !state.Config.Auth.PasswordLoginEnabled || !state.Config.Auth.PhoneLoginEnabled {
				return nil, errors.New("后台已关闭手机号注册")
			}
			if !phoneRe.MatchString(phone) {
				return nil, errors.New("请输入 11 位手机号")
			}
			if err := ensureSMSCode(state.Config.SMSLogs, phone, payload.SMSCode, "register"); err != nil {
				return nil, err
			}
		} else {
			if !state.Config.Auth.PasswordLoginEnabled || !state.Config.Auth.EmailLoginEnabled {
				return nil, errors.New("后台已关闭邮箱注册")
			}
			phone = normalizePhone(payload.Phone)
			if phone != "" && !phoneRe.MatchString(phone) {
				return nil, errors.New("请输入 11 位手机号")
			}
		}
		invite := findInvite(state.Config.Invites, payload.InviteCode)
		if state.Config.Auth.InviteRequired && invite == nil {
			return nil, errors.New("请填写有效注册码")
		}
		if invite != nil {
			if invite.UsedAt != "" {
				return nil, errors.New("这个注册码已被使用")
			}
			if invite.ExpiresAt != "" {
				expiresAt, err := time.Parse(time.RFC3339, invite.ExpiresAt)
				if err == nil && time.Now().After(expiresAt) {
					return nil, errors.New("这个注册码已过期")
				}
			}
			if invite.Phone != "" && invite.Phone != phone {
				if phone == "" {
					return nil, errors.New("这个注册码绑定了手机号，请使用手机号注册")
				}
				return nil, errors.New("这个注册码不属于当前手机号")
			}
		}
		localPart := ""
		var err error
		if invite != nil && invite.MailboxLocalPart != "" {
			localPart = invite.MailboxLocalPart
		} else if loginMode == "email" && strings.TrimSpace(payload.EmailPrefix) == "" {
			localPart, err = mailboxLocalPartFromInput(state.Config.Mailbox, firstNonEmpty(payload.Email, payload.Identifier))
			if err != nil {
				return nil, err
			}
		} else {
			localPart, err = buildMailboxLocalPart(state.Config.Mailbox, payload.EmailPrefix, payload.Prefix)
			if err != nil {
				return nil, err
			}
		}
		email := localPart + "@" + state.Config.Mailbox.Domain
		for _, user := range state.Users {
			if strings.EqualFold(user.Email, email) || (phone != "" && user.Phone == phone) {
				return nil, errors.New("手机号或邮箱已注册")
			}
		}
		passwordHash, err := hashPassword(payload.Password)
		if err != nil {
			return nil, err
		}
		createdAt := nowISO()
		user := UserRecord{
			RegisteredUser: RegisteredUser{
				ID:           nextID("account"),
				Phone:        phone,
				Email:        email,
				DisplayName:  firstNonEmpty(strings.TrimSpace(payload.DisplayName), "MyName"),
				RegisteredAt: createdAt,
				Source:       sourceLabel(invite != nil, loginMode+"_register"),
				Status:       "active",
			},
			PasswordHash: passwordHash,
		}
		applyMailboxProvisionState(&user, state.Config.Mailbox.Server)
		queueProvisionJob(state, user, createdAt)
		state.Users = append([]UserRecord{user}, state.Users...)
		state.Config.RegisteredUsers = append([]RegisteredUser{user.RegisteredUser}, state.Config.RegisteredUsers...)
		if invite != nil {
			markInviteUsed(state.Config.Invites, invite.Code, createdAt)
		}
		state.Settings[user.ID] = defaultSettings(user)
		state.Messages[user.ID] = []MailMessage{}
		session := createSessionForRequest(user.ID, r)
		putSession(state.Sessions, session)
		appendAuditLog(state, r, "user", "auth.register", user.Email, user.Source)
		state.Config.UpdatedAt = createdAt
		return sessionPayload(session.Token, session, user, state.Config), nil
	})
	if err != nil {
		return domainError(err)
	}
	setSessionCookieFromPayload(r, result)
	return http.StatusOK, result
}

func (app *App) loginWithPassword(r *http.Request) (int, any) {
	var payload struct {
		LoginType  string `json:"loginType"`
		Identifier string `json:"identifier"`
		Phone      string `json:"phone"`
		Email      string `json:"email"`
		SMSCode    string `json:"smsCode"`
		Password   string `json:"password"`
	}
	if err := decodeJSON(r, &payload); err != nil {
		return badRequest("invalid json body")
	}

	result, err := app.store.mutate(func(state *AppState) (any, error) {
		state.Config.Auth = normalizeAuthConfig(state.Config.Auth)
		identifier := firstNonEmpty(payload.Identifier, payload.Email, payload.Phone)
		loginMode := normalizeLoginMode(payload.LoginType, identifier, state.Config.Auth)
		phone := normalizePhone(firstNonEmpty(payload.Phone, payload.Identifier))
		email := ""
		if loginMode == "phone" {
			if !state.Config.Auth.PasswordLoginEnabled || !state.Config.Auth.PhoneLoginEnabled {
				return nil, errors.New("后台已关闭手机号登录")
			}
			if !phoneRe.MatchString(phone) {
				return nil, errors.New("请输入 11 位手机号")
			}
			if err := ensureSMSCode(state.Config.SMSLogs, phone, payload.SMSCode, "login"); err != nil {
				return nil, err
			}
		} else {
			if !state.Config.Auth.PasswordLoginEnabled || !state.Config.Auth.EmailLoginEnabled {
				return nil, errors.New("后台已关闭邮箱登录")
			}
			var err error
			email, err = mailboxEmailFromInput(state.Config.Mailbox, firstNonEmpty(payload.Email, payload.Identifier))
			if err != nil {
				return nil, err
			}
		}
		for _, user := range state.Users {
			matchesIdentity := false
			if loginMode == "phone" {
				matchesIdentity = user.Phone == phone
			} else {
				matchesIdentity = strings.EqualFold(user.Email, email)
			}
			if matchesIdentity && verifyPassword(user.PasswordHash, payload.Password) {
				if !accountActive(user.RegisteredUser) {
					return nil, errors.New("账号已被禁用，请联系管理员")
				}
				session := createSessionForRequest(user.ID, r)
				putSession(state.Sessions, session)
				appendAuditLog(state, r, "user", "auth.login", user.Email, map[string]string{"phone": "手机号密码登录", "email": "邮箱密码登录"}[loginMode])
				return sessionPayload(session.Token, session, user, state.Config), nil
			}
		}
		return nil, errors.New("账号或密码不正确")
	})
	if err != nil {
		return domainError(err)
	}
	setSessionCookieFromPayload(r, result)
	return http.StatusOK, result
}

func (app *App) beginOAuth(r *http.Request) (int, any) {
	state := app.store.snapshot()
	auth := normalizeAuthConfig(state.Config.Auth)
	if !auth.OAuthEnabled {
		return domainError(errors.New("后台已关闭 OAuth 登录"))
	}
	if oauthConfigComplete(auth) {
		stateToken := "oauth_" + randomHex(24)
		setOAuthStateCookie(responseWriterFromRequest(r), stateToken)
		redirectURL, err := buildOAuthAuthorizeRedirect(auth, stateToken)
		if err != nil {
			return domainError(err)
		}
		return http.StatusOK, response{
			"redirectUrl":         redirectURL,
			"redirected":          false,
			"isAuthenticated":     false,
			"requiresActivation":  false,
			"rolePrefix":          defaultRolePrefix(state.Config.Mailbox),
			"adminConfig":         publicAdminConfig(state.Config),
			"oauthProviderName":   auth.OAuthProviderName,
			"oauthProductionFlow": true,
		}
	}
	return domainError(errors.New("OAuth Provider 未完整配置，请先在后台保存真实 OAuth/OIDC 配置"))
}

func (app *App) finishOAuth(w http.ResponseWriter, r *http.Request) {
	state := app.store.snapshot()
	auth := normalizeAuthConfig(state.Config.Auth)
	redirectWithError := func(message string) {
		http.Redirect(w, r, oauthReturnURL(app.userAppOrigin, "oauth_error", message), http.StatusFound)
	}
	if !auth.OAuthEnabled {
		redirectWithError("后台已关闭 OAuth 登录")
		return
	}
	if !oauthConfigComplete(auth) {
		redirectWithError("OAuth Provider 未完整配置")
		return
	}
	if providerError := strings.TrimSpace(r.URL.Query().Get("error")); providerError != "" {
		redirectWithError(providerError)
		return
	}
	code := strings.TrimSpace(r.URL.Query().Get("code"))
	if code == "" {
		redirectWithError("OAuth 回调缺少 code")
		return
	}
	queryState := strings.TrimSpace(r.URL.Query().Get("state"))
	cookieState := ""
	if cookie, err := r.Cookie(oauthStateCookieName); err == nil {
		cookieState = strings.TrimSpace(cookie.Value)
	}
	clearOAuthStateCookie(w)
	if queryState == "" || cookieState == "" || !constantTimeStringEqual(queryState, cookieState) {
		redirectWithError("OAuth state 校验失败")
		return
	}
	token, err := exchangeOAuthCode(r.Context(), auth, code)
	if err != nil {
		redirectWithError(err.Error())
		return
	}
	principal, err := fetchOAuthPrincipal(r.Context(), auth, token)
	if err != nil {
		redirectWithError(err.Error())
		return
	}
	result, err := app.upsertOAuthSession(r, principal)
	if err != nil {
		redirectWithError(err.Error())
		return
	}
	payload, ok := result.(response)
	if !ok {
		redirectWithError("OAuth 登录结果无效")
		return
	}
	tokenValue, _ := payload["token"].(string)
	if tokenValue != "" {
		setSessionCookie(w, tokenValue)
	}
	http.Redirect(w, r, oauthReturnURL(app.userAppOrigin, "oauth", "ok"), http.StatusFound)
}

func buildOAuthAuthorizeRedirect(auth AuthConfig, stateToken string) (string, error) {
	auth = normalizeAuthConfig(auth)
	parsed, err := url.Parse(auth.OAuthAuthorizeURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", errors.New("OAuth 授权地址无效")
	}
	query := parsed.Query()
	query.Set("response_type", "code")
	query.Set("client_id", auth.OAuthClientID)
	query.Set("redirect_uri", auth.OAuthRedirectURL)
	query.Set("state", stateToken)
	query.Set("scope", strings.Join(auth.OAuthScopes, " "))
	parsed.RawQuery = query.Encode()
	return parsed.String(), nil
}

func exchangeOAuthCode(ctx context.Context, auth AuthConfig, code string) (string, error) {
	auth = normalizeAuthConfig(auth)
	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("code", code)
	form.Set("redirect_uri", auth.OAuthRedirectURL)
	form.Set("client_id", auth.OAuthClientID)
	form.Set("client_secret", auth.OAuthClientSecret)

	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, auth.OAuthTokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", errors.New("OAuth Token 地址无效")
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("OAuth Token 接口调用失败: %w", err)
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(res.Body, 128*1024))
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		message := strings.TrimSpace(string(body))
		if message == "" {
			message = http.StatusText(res.StatusCode)
		}
		return "", fmt.Errorf("OAuth Token 接口返回 HTTP %d: %s", res.StatusCode, message)
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return "", errors.New("OAuth Token 返回不是有效 JSON")
	}
	accessToken := stringFromAny(payload["access_token"])
	if accessToken == "" {
		return "", errors.New("OAuth Token 返回缺少 access_token")
	}
	return accessToken, nil
}

func fetchOAuthPrincipal(ctx context.Context, auth AuthConfig, accessToken string) (OAuthPrincipal, error) {
	auth = normalizeAuthConfig(auth)
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, auth.OAuthUserInfoURL, nil)
	if err != nil {
		return OAuthPrincipal{}, errors.New("OAuth 用户信息地址无效")
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+accessToken)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return OAuthPrincipal{}, fmt.Errorf("OAuth 用户信息接口调用失败: %w", err)
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(res.Body, 128*1024))
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		message := strings.TrimSpace(string(body))
		if message == "" {
			message = http.StatusText(res.StatusCode)
		}
		return OAuthPrincipal{}, fmt.Errorf("OAuth 用户信息接口返回 HTTP %d: %s", res.StatusCode, message)
	}
	var raw map[string]any
	if err := json.Unmarshal(body, &raw); err != nil {
		return OAuthPrincipal{}, errors.New("OAuth 用户信息返回不是有效 JSON")
	}
	principal := OAuthPrincipal{
		Subject:     firstNonEmpty(jsonFieldString(raw, auth.OAuthSubjectField), jsonFieldString(raw, "sub"), jsonFieldString(raw, "id"), jsonFieldString(raw, "openid"), jsonFieldString(raw, "unionid")),
		Phone:       normalizePhone(firstNonEmpty(jsonFieldString(raw, auth.OAuthPhoneField), jsonFieldString(raw, "phone"), jsonFieldString(raw, "mobile"), jsonFieldString(raw, "phone_number"))),
		Email:       strings.ToLower(firstNonEmpty(jsonFieldString(raw, auth.OAuthEmailField), jsonFieldString(raw, "email"))),
		DisplayName: firstNonEmpty(jsonFieldString(raw, auth.OAuthNameField), jsonFieldString(raw, "name"), jsonFieldString(raw, "nickname"), jsonFieldString(raw, "display_name")),
		Raw:         raw,
	}
	if principal.Subject == "" {
		principal.Subject = firstNonEmpty(principal.Email, principal.Phone)
	}
	if principal.Subject == "" || (principal.Email == "" && principal.Phone == "") {
		return OAuthPrincipal{}, errors.New("OAuth 用户信息缺少可识别的手机号或邮箱")
	}
	if principal.Phone != "" && !phoneRe.MatchString(principal.Phone) {
		principal.Phone = ""
	}
	principal.DisplayName = firstNonEmpty(principal.DisplayName, "OAuth 用户")
	return principal, nil
}

func (app *App) upsertOAuthSession(r *http.Request, principal OAuthPrincipal) (any, error) {
	return app.store.mutate(func(state *AppState) (any, error) {
		state.Config.Auth = normalizeAuthConfig(state.Config.Auth)
		if !state.Config.Auth.OAuthEnabled {
			return nil, errors.New("后台已关闭 OAuth 登录")
		}
		for _, user := range state.Users {
			if (principal.Phone != "" && user.Phone == principal.Phone) || (principal.Email != "" && strings.EqualFold(user.Email, principal.Email)) {
				if !accountActive(user.RegisteredUser) {
					return nil, errors.New("账号已被禁用，请联系管理员")
				}
				session := createSessionForRequest(user.ID, r)
				putSession(state.Sessions, session)
				appendAuditLog(state, r, "user", "auth.oauth.login", user.Email, state.Config.Auth.OAuthProviderName)
				return sessionPayload(session.Token, session, user, state.Config), nil
			}
		}

		localPart, err := oauthMailboxLocalPart(state.Config.Mailbox, principal)
		if err != nil {
			return nil, err
		}
		email := localPart + "@" + normalizeDomain(state.Config.Mailbox.Domain)
		if emailExists(state.Users, email) {
			return nil, errors.New("OAuth 映射邮箱已存在，请联系管理员处理")
		}
		passwordHash, err := hashPassword(randomHex(24))
		if err != nil {
			return nil, err
		}
		createdAt := nowISO()
		user := UserRecord{
			RegisteredUser: RegisteredUser{
				ID:           nextID("oauth"),
				Phone:        principal.Phone,
				Email:        email,
				DisplayName:  principal.DisplayName,
				RegisteredAt: createdAt,
				Source:       "oauth",
				Status:       "active",
			},
			PasswordHash: passwordHash,
		}
		applyMailboxProvisionState(&user, state.Config.Mailbox.Server)
		queueProvisionJob(state, user, createdAt)
		state.Users = append([]UserRecord{user}, state.Users...)
		state.Config.RegisteredUsers = append([]RegisteredUser{user.RegisteredUser}, state.Config.RegisteredUsers...)
		state.Settings[user.ID] = defaultSettings(user)
		state.Messages[user.ID] = []MailMessage{}
		session := createSessionForRequest(user.ID, r)
		putSession(state.Sessions, session)
		appendAuditLog(state, r, "user", "auth.oauth.register", user.Email, state.Config.Auth.OAuthProviderName)
		return sessionPayload(session.Token, session, user, state.Config), nil
	})
}

func oauthMailboxLocalPart(config MailboxConfig, principal OAuthPrincipal) (string, error) {
	if principal.Email != "" {
		parts := strings.SplitN(strings.ToLower(strings.TrimSpace(principal.Email)), "@", 2)
		if len(parts) == 2 && strings.EqualFold(normalizeDomain(parts[1]), normalizeDomain(config.Domain)) {
			return mailboxLocalPartFromInput(config, principal.Email)
		}
	}
	seed := firstNonEmpty(principal.Phone, principal.Subject, principal.DisplayName, "oauth")
	if strings.Contains(seed, "@") {
		seed = strings.SplitN(seed, "@", 2)[0]
	}
	return buildMailboxLocalPart(config, seed, config.DefaultPrefix)
}

func jsonFieldString(raw map[string]any, path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	current := any(raw)
	for _, part := range strings.Split(path, ".") {
		object, ok := current.(map[string]any)
		if !ok {
			return ""
		}
		current = object[strings.TrimSpace(part)]
	}
	return stringFromAny(current)
}

func oauthReturnURL(base string, key string, value string) string {
	base = firstNonEmpty(strings.TrimSpace(base), "http://127.0.0.1:1788")
	parsed, err := url.Parse(base)
	if err != nil {
		return base
	}
	query := parsed.Query()
	query.Set(key, value)
	parsed.RawQuery = query.Encode()
	return parsed.String()
}

func (app *App) getSession(r *http.Request) (int, any) {
	state := app.store.snapshot()
	token := sessionToken(r)
	if token == "" {
		return http.StatusOK, response{
			"isAuthenticated":    false,
			"requiresActivation": true,
			"rolePrefix":         defaultRolePrefix(state.Config.Mailbox),
			"adminConfig":        publicAdminConfig(state.Config),
		}
	}
	session, ok := lookupSession(state.Sessions, token)
	if !ok || sessionExpired(session) {
		return http.StatusOK, response{
			"isAuthenticated":    false,
			"requiresActivation": true,
			"rolePrefix":         defaultRolePrefix(state.Config.Mailbox),
			"adminConfig":        publicAdminConfig(state.Config),
		}
	}
	user, ok := findUserByID(state.Users, session.UserID)
	if !ok {
		return http.StatusOK, response{"isAuthenticated": false, "requiresActivation": true, "rolePrefix": defaultRolePrefix(state.Config.Mailbox), "adminConfig": publicAdminConfig(state.Config)}
	}
	if !accountActive(user.RegisteredUser) {
		return http.StatusOK, response{"isAuthenticated": false, "requiresActivation": true, "rolePrefix": defaultRolePrefix(state.Config.Mailbox), "adminConfig": publicAdminConfig(state.Config), "message": "账号已被禁用，请联系管理员"}
	}
	payload := sessionPayload(token, session, user, state.Config)
	return http.StatusOK, payload
}

func (app *App) logout(r *http.Request) (int, any) {
	token := sessionToken(r)
	if token != "" {
		_, _ = app.store.mutate(func(state *AppState) (any, error) {
			delete(state.Sessions, sessionStorageKey(token))
			delete(state.Sessions, token)
			return response{"success": true}, nil
		})
	}
	clearSessionCookie(r)
	return http.StatusOK, response{"success": true}
}

func (app *App) listSecuritySessions(r *http.Request) (int, any) {
	token := sessionToken(r)
	if token == "" {
		return unauthorized()
	}
	result, err := app.store.mutate(func(state *AppState) (any, error) {
		currentKey, currentSession, user, ok := authenticatedSessionDetails(token, state)
		if !ok {
			return nil, errUnauthorized
		}
		updateSessionRequestMetadata(&currentSession, r, nowISO())
		state.Sessions[currentKey] = normalizeSessionRecord(currentKey, currentSession)
		return envelope(response{
			"items": securitySessionItems(state.Sessions, user.ID, currentKey),
		}), nil
	})
	if err != nil {
		if errors.Is(err, errUnauthorized) {
			return unauthorized()
		}
		return domainError(err)
	}
	return http.StatusOK, result
}

func (app *App) logoutOtherSecuritySessions(r *http.Request) (int, any) {
	token := sessionToken(r)
	if token == "" {
		return unauthorized()
	}
	result, err := app.store.mutate(func(state *AppState) (any, error) {
		currentKey, currentSession, user, ok := authenticatedSessionDetails(token, state)
		if !ok {
			return nil, errUnauthorized
		}
		updateSessionRequestMetadata(&currentSession, r, nowISO())
		state.Sessions[currentKey] = normalizeSessionRecord(currentKey, currentSession)

		removed := 0
		for key, session := range state.Sessions {
			session = normalizeSessionRecord(key, session)
			if session.UserID != user.ID || key == currentKey || session.ID == currentSession.ID {
				continue
			}
			delete(state.Sessions, key)
			removed++
		}
		appendAuditLog(state, r, "user", "auth.sessions.logout_others", user.Email, fmt.Sprintf("退出 %d 个其他登录设备", removed))
		return envelope(response{
			"success": true,
			"removed": removed,
			"items":   securitySessionItems(state.Sessions, user.ID, currentKey),
		}), nil
	})
	if err != nil {
		if errors.Is(err, errUnauthorized) {
			return unauthorized()
		}
		return domainError(err)
	}
	return http.StatusOK, result
}

func (app *App) revokeSecuritySession(r *http.Request) (int, any) {
	token := sessionToken(r)
	sessionID := strings.TrimSpace(r.PathValue("sessionID"))
	if token == "" {
		return unauthorized()
	}
	if sessionID == "" {
		return badRequest("missing session id")
	}
	result, err := app.store.mutate(func(state *AppState) (any, error) {
		currentKey, currentSession, user, ok := authenticatedSessionDetails(token, state)
		if !ok {
			return nil, errUnauthorized
		}
		updateSessionRequestMetadata(&currentSession, r, nowISO())
		state.Sessions[currentKey] = normalizeSessionRecord(currentKey, currentSession)
		if sessionID == currentSession.ID {
			return nil, errors.New("当前设备请使用退出登录")
		}

		removed := 0
		for key, session := range state.Sessions {
			session = normalizeSessionRecord(key, session)
			if session.UserID == user.ID && session.ID == sessionID {
				delete(state.Sessions, key)
				removed++
			}
		}
		if removed == 0 {
			return nil, errors.New("登录设备不存在或已失效")
		}
		appendAuditLog(state, r, "user", "auth.sessions.revoke", user.Email, "移除登录设备 "+sessionID)
		return envelope(response{
			"success": true,
			"removed": removed,
			"items":   securitySessionItems(state.Sessions, user.ID, currentKey),
		}), nil
	})
	if err != nil {
		if errors.Is(err, errUnauthorized) {
			return unauthorized()
		}
		return domainError(err)
	}
	return http.StatusOK, result
}

func (app *App) getMailboxProfile(r *http.Request) (int, any) {
	state := app.store.snapshot()
	user, ok := authenticatedUser(r, state)
	if !ok {
		return unauthorized()
	}
	return http.StatusOK, envelope(response{
		"profile":      buildProfile(user, state.Config.Mailbox),
		"folderCounts": folderCounts(state.Messages[user.ID]),
	})
}

func (app *App) activateMailbox(r *http.Request) (int, any) {
	var payload struct {
		EmailPrefix string `json:"emailPrefix"`
		Prefix      string `json:"prefix"`
	}
	if err := decodeJSON(r, &payload); err != nil {
		return badRequest("invalid json body")
	}
	token := sessionToken(r)
	result, err := app.store.mutate(func(state *AppState) (any, error) {
		userIndex, ok := authenticatedUserIndex(token, state)
		if !ok {
			return nil, errUnauthorized
		}
		localPart, err := buildMailboxLocalPart(state.Config.Mailbox, payload.EmailPrefix, payload.Prefix)
		if err != nil {
			return nil, err
		}
		email := localPart + "@" + state.Config.Mailbox.Domain
		for index, user := range state.Users {
			if index != userIndex && strings.EqualFold(user.Email, email) {
				return nil, errors.New("邮箱已被注册")
			}
		}
		nowValue := nowISO()
		user := &state.Users[userIndex]
		user.Email = email
		applyMailboxProvisionState(user, state.Config.Mailbox.Server)
		job := queueProvisionJob(state, *user, nowValue)
		jobIndex := -1
		for index := range state.Config.ProvisionJobs {
			if state.Config.ProvisionJobs[index].ID == job.ID {
				jobIndex = index
				break
			}
		}
		var provision MailboxProvisionResult
		if jobIndex >= 0 {
			jobPtr := &state.Config.ProvisionJobs[jobIndex]
			server := normalizeMailServerConfig(state.Config.Mailbox.Server)
			if server.Enabled && server.BaseURL != "" && server.Status == "online" {
				jobPtr.Status = "running"
				jobPtr.Attempts += 1
				jobPtr.LastRunAt = nowValue
				jobPtr.UpdatedAt = nowValue
				provisioned, provisionErr := provisionMailboxAccount(r.Context(), state.Config, *user, *jobPtr)
				state.Config.Mailbox.Server.LastProvisionCheckAt = nowValue
				if provisionErr != nil {
					jobPtr.Status = "failed"
					jobPtr.LastError = provisionErr.Error()
					jobPtr.NextRunAt = time.Now().Add(provisionRetryDelay(jobPtr.Attempts)).Format(time.RFC3339)
					jobPtr.UpdatedAt = nowValue
					user.MailboxStatus = "failed"
					user.MailboxLastError = jobPtr.LastError
					user.MailboxProvisionedAt = ""
				} else {
					provision = provisioned
					jobPtr.Status = "succeeded"
					jobPtr.LastError = ""
					jobPtr.NextRunAt = ""
					jobPtr.CompletedAt = nowValue
					jobPtr.UpdatedAt = nowValue
					user.MailboxStatus = "provisioned"
					user.MailboxProvisionedAt = nowValue
					user.MailboxExternalID = firstNonEmpty(provisioned.ExternalID, user.MailboxExternalID, "remote_"+user.ID)
					user.MailboxLastError = ""
				}
			}
			job = *jobPtr
		}
		updateRegisteredUser(state.Config.RegisteredUsers, user.RegisteredUser)
		appendAuditLog(state, r, "user", "mailbox.activate", user.Email, mailboxProvisionMessage(user))
		state.Config.UpdatedAt = nowValue
		return envelope(response{"mailbox": buildProfile(*user, state.Config.Mailbox), "job": job, "provision": provision, "message": mailboxProvisionMessage(user)}), nil
	})
	if err != nil {
		if errors.Is(err, errUnauthorized) {
			return unauthorized()
		}
		return domainError(err)
	}
	return http.StatusOK, result
}

func (app *App) listMessages(r *http.Request) (int, any) {
	state := app.store.snapshot()
	user, ok := authenticatedUser(r, state)
	if !ok {
		return unauthorized()
	}
	if err := ensureProvisionedMailbox(user); err != nil {
		return domainError(err)
	}
	folderID := strings.TrimSpace(r.URL.Query().Get("folderId"))
	if folderID == "" {
		folderID = "inbox"
	}
	filter := strings.TrimSpace(r.URL.Query().Get("filter"))
	search := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("search")))

	remoteList, err := fetchMailboxMessageList(r.Context(), state.Config, user, folderID, filter, search)
	if err != nil {
		return domainError(err)
	}
	if remoteList.Called {
		for index := range remoteList.Items {
			attachments, err := app.persistAttachmentList(user.ID, remoteList.Items[index].Attachments, app.apiBasePath(r))
			if err != nil {
				return domainError(err)
			}
			remoteList.Items[index].Attachments = attachments
			remoteList.Items[index].HasAttachment = remoteList.Items[index].HasAttachment || len(attachments) > 0
		}
		token := sessionToken(r)
		result, err := app.store.mutate(func(state *AppState) (any, error) {
			userIndex, ok := authenticatedUserIndex(token, state)
			if !ok {
				return nil, errUnauthorized
			}
			accountID := state.Users[userIndex].ID
			for _, message := range remoteList.Items {
				state.Messages[accountID] = upsertUserMessage(state.Messages[accountID], message)
			}
			messages := filterMessages(state.Messages[accountID], folderID, filter, search)
			return envelope(response{
				"items":        messages,
				"folderCounts": folderCounts(state.Messages[accountID]),
				"hasMore":      remoteList.HasMore,
				"nextCursor":   remoteList.NextCursor,
			}), nil
		})
		if err != nil {
			return domainError(err)
		}
		return http.StatusOK, result
	}

	return domainError(missingMailDataPlaneEndpointError("收件列表接口"))
}

func (app *App) getMessageDetail(r *http.Request) (int, any) {
	state := app.store.snapshot()
	user, ok := authenticatedUser(r, state)
	if !ok {
		return unauthorized()
	}
	if err := ensureProvisionedMailbox(user); err != nil {
		return domainError(err)
	}
	messageID := strings.TrimSpace(r.PathValue("messageID"))
	remoteMessage, remoteCalled, err := fetchMailboxMessageDetail(r.Context(), state.Config, user, messageID)
	if err != nil {
		return domainError(err)
	}
	if remoteCalled {
		attachments, err := app.persistAttachmentList(user.ID, remoteMessage.Attachments, app.apiBasePath(r))
		if err != nil {
			return domainError(err)
		}
		remoteMessage.Attachments = attachments
		remoteMessage.HasAttachment = remoteMessage.HasAttachment || len(attachments) > 0
		token := sessionToken(r)
		result, err := app.store.mutate(func(state *AppState) (any, error) {
			userIndex, ok := authenticatedUserIndex(token, state)
			if !ok {
				return nil, errUnauthorized
			}
			accountID := state.Users[userIndex].ID
			state.Messages[accountID] = upsertUserMessage(state.Messages[accountID], remoteMessage)
			return envelope(response{"message": remoteMessage}), nil
		})
		if err != nil {
			return domainError(err)
		}
		return http.StatusOK, result
	}
	return domainError(missingMailDataPlaneEndpointError("邮件详情接口"))
}

func (app *App) sendMessage(r *http.Request) (int, any) {
	var payload SendMessagePayload
	if err := decodeJSON(r, &payload); err != nil {
		return badRequest("invalid json body")
	}
	payload, err := normalizeSendMessagePayload(payload)
	if err != nil {
		return domainError(err)
	}
	token := sessionToken(r)
	state := app.store.snapshot()
	user, ok := authenticatedUser(r, state)
	if !ok {
		return unauthorized()
	}
	if err := ensureProvisionedMailbox(user); err != nil {
		return domainError(err)
	}
	relayAttachments, err := app.hydrateAttachmentList(user.ID, payload.Attachments)
	if err != nil {
		return domainError(err)
	}
	storedAttachments, err := app.persistAttachmentList(user.ID, payload.Attachments, app.apiBasePath(r))
	if err != nil {
		return domainError(err)
	}
	relayPayload := payload
	relayPayload.Attachments = relayAttachments
	payload.Attachments = storedAttachments
	now := time.Now()
	message := buildOutgoingMessage(user, payload, now)
	remoteResult, err := relayMailboxMessage(r.Context(), state.Config, user, message, relayPayload, SaveDraftPayload{}, "send")
	if err != nil {
		return domainError(err)
	}
	message = mergeRelayMessage(message, remoteResult.Message)
	message.Attachments, err = app.persistAttachmentList(user.ID, message.Attachments, app.apiBasePath(r))
	if err != nil {
		return domainError(err)
	}
	acceptedAt := firstNonEmpty(remoteResult.AcceptedAt, message.SentAt, now.Format(time.RFC3339))
	providerMessageID := remoteResult.ProviderMessageID
	message.AcceptedAt = acceptedAt
	message.ProviderMessageID = providerMessageID
	message.DeliveryError = ""
	message.DeliveryStatus = firstNonEmpty(message.DeliveryStatus, "accepted")
	result, err := app.store.mutate(func(state *AppState) (any, error) {
		userIndex, ok := authenticatedUserIndex(token, state)
		if !ok {
			return nil, errUnauthorized
		}
		accountID := state.Users[userIndex].ID
		state.Messages[accountID] = upsertUserMessage(state.Messages[accountID], message)
		appendAuditLog(state, r, "user", "message.send", message.ID, "用户发送邮件")
		return envelope(response{
			"message":           message,
			"acceptedAt":        acceptedAt,
			"providerMessageId": nullableString(providerMessageID),
		}), nil
	})
	if err != nil {
		return domainError(err)
	}
	return http.StatusOK, result
}

func (app *App) saveDraft(r *http.Request) (int, any) {
	var payload SaveDraftPayload
	if err := decodeJSON(r, &payload); err != nil {
		return badRequest("invalid json body")
	}
	payload, err := normalizeSaveDraftPayload(payload)
	if err != nil {
		return domainError(err)
	}
	token := sessionToken(r)
	state := app.store.snapshot()
	user, ok := authenticatedUser(r, state)
	if !ok {
		return unauthorized()
	}
	if err := ensureProvisionedMailbox(user); err != nil {
		return domainError(err)
	}
	relayAttachments, err := app.hydrateAttachmentList(user.ID, payload.Attachments)
	if err != nil {
		return domainError(err)
	}
	storedAttachments, err := app.persistAttachmentList(user.ID, payload.Attachments, app.apiBasePath(r))
	if err != nil {
		return domainError(err)
	}
	relayPayload := payload
	relayPayload.Attachments = relayAttachments
	payload.Attachments = storedAttachments
	draft := buildDraftMessage(user, payload, time.Now())
	remoteResult, err := relayMailboxMessage(r.Context(), state.Config, user, draft, SendMessagePayload{}, relayPayload, "draft")
	if err != nil {
		return domainError(err)
	}
	draft = mergeRelayMessage(draft, remoteResult.Draft)
	draft.Attachments, err = app.persistAttachmentList(user.ID, draft.Attachments, app.apiBasePath(r))
	if err != nil {
		return domainError(err)
	}
	result, err := app.store.mutate(func(state *AppState) (any, error) {
		userIndex, ok := authenticatedUserIndex(token, state)
		if !ok {
			return nil, errUnauthorized
		}
		accountID := state.Users[userIndex].ID
		state.Messages[accountID] = upsertUserMessage(state.Messages[accountID], draft)
		appendAuditLog(state, r, "user", "message.draft.save", draft.ID, "用户保存草稿")
		return envelope(response{"draft": draft}), nil
	})
	if err != nil {
		return domainError(err)
	}
	return http.StatusOK, result
}

func (app *App) uploadAttachments(r *http.Request) (int, any) {
	var payload struct {
		Attachments []any `json:"attachments"`
		Attachment  any   `json:"attachment"`
	}
	if err := decodeJSON(r, &payload); err != nil {
		return badRequest("invalid json body")
	}
	state := app.store.snapshot()
	user, ok := authenticatedUser(r, state)
	if !ok {
		return unauthorized()
	}
	if err := ensureProvisionedMailbox(user); err != nil {
		return domainError(err)
	}
	attachments := payload.Attachments
	if len(attachments) == 0 && payload.Attachment != nil {
		attachments = []any{payload.Attachment}
	}
	attachments, err := normalizeAttachmentList(attachments)
	if err != nil {
		return domainError(err)
	}
	items, err := app.persistAttachmentList(user.ID, attachments, app.apiBasePath(r))
	if err != nil {
		return domainError(err)
	}
	if len(items) > 0 {
		_, _ = app.store.mutate(func(state *AppState) (any, error) {
			appendAuditLog(state, r, "user", "attachment.upload", user.Email, fmt.Sprintf("%d attachments", len(items)))
			return nil, nil
		})
	}
	return http.StatusOK, envelope(response{"items": items})
}

func (app *App) downloadAttachment(w http.ResponseWriter, r *http.Request) {
	state := app.store.snapshot()
	user, ok := authenticatedUser(r, state)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, response{"message": "请先登录"})
		return
	}
	if err := ensureProvisionedMailbox(user); err != nil {
		writeJSON(w, http.StatusBadRequest, response{"message": err.Error()})
		return
	}
	assetID := strings.TrimSpace(r.PathValue("assetID"))
	record, filePath, err := app.loadAttachmentAsset(user.ID, assetID)
	if err != nil {
		writeJSON(w, http.StatusNotFound, response{"message": "附件不存在或无权访问"})
		return
	}
	file, err := os.Open(filePath)
	if err != nil {
		writeJSON(w, http.StatusNotFound, response{"message": "附件文件不存在"})
		return
	}
	defer file.Close()
	w.Header().Set("Content-Type", firstNonEmpty(record.ContentType, "application/octet-stream"))
	w.Header().Set("Content-Length", strconv.FormatInt(record.SizeBytes, 10))
	w.Header().Set("Cache-Control", "private, max-age=300")
	w.Header().Set("Content-Disposition", contentDispositionAttachment(record.Name))
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, file)
}

func (app *App) listContacts(r *http.Request) (int, any) {
	state := app.store.snapshot()
	user, ok := authenticatedUser(r, state)
	if !ok {
		return unauthorized()
	}
	if err := ensureProvisionedMailbox(user); err != nil {
		return domainError(err)
	}
	messages, err := app.contactSourceMessages(r.Context(), user, state)
	if err != nil {
		return domainError(err)
	}
	contacts := buildContactsFromMessages(user, messages, strings.TrimSpace(r.URL.Query().Get("search")))
	return http.StatusOK, envelope(response{"items": contacts})
}

func (app *App) getContactThread(r *http.Request) (int, any) {
	state := app.store.snapshot()
	user, ok := authenticatedUser(r, state)
	if !ok {
		return unauthorized()
	}
	if err := ensureProvisionedMailbox(user); err != nil {
		return domainError(err)
	}
	contactID := strings.TrimSpace(r.PathValue("contactID"))
	if contactID == "" {
		return badRequest("联系人不存在")
	}
	messages, err := app.contactSourceMessages(r.Context(), user, state)
	if err != nil {
		return domainError(err)
	}
	contacts := buildContactsFromMessages(user, messages, "")
	var selected ContactRecord
	for _, contact := range contacts {
		if contact.ID == contactID || strings.EqualFold(contact.Email, contactID) {
			selected = contact
			break
		}
	}
	if selected.ID == "" {
		return badRequest("联系人不存在")
	}
	items := contactThreadItems(messages, selected)
	return http.StatusOK, envelope(response{
		"contact": selected,
		"items":   items,
		"total":   len(items),
	})
}

func (app *App) listTemplates(r *http.Request) (int, any) {
	state := app.store.snapshot()
	user, ok := authenticatedUser(r, state)
	if !ok {
		return unauthorized()
	}
	if err := ensureProvisionedMailbox(user); err != nil {
		return domainError(err)
	}
	return http.StatusOK, envelope(response{"items": buildMailTemplates(user)})
}

func (app *App) sendTemplateMessage(r *http.Request) (int, any) {
	var payload TemplateSendPayload
	if err := decodeJSON(r, &payload); err != nil {
		return badRequest("invalid json body")
	}
	recipients, err := normalizeEmailList(payload.Recipients, true)
	if err != nil {
		return domainError(err)
	}
	token := sessionToken(r)
	state := app.store.snapshot()
	user, ok := authenticatedUser(r, state)
	if !ok {
		return unauthorized()
	}
	if err := ensureProvisionedMailbox(user); err != nil {
		return domainError(err)
	}
	template := selectMailTemplate(buildMailTemplates(user), payload.Role)
	sendPayload, err := normalizeSendMessagePayload(SendMessagePayload{
		Recipients:  recipients,
		Subject:     template.Subject,
		Body:        MessageBodyPayload{Format: "html", HTML: template.HTML, Text: stripHTML(template.HTML)},
		TemplateID:  template.ID,
		Source:      "template",
		Attachments: []any{},
	})
	if err != nil {
		return domainError(err)
	}
	now := time.Now()
	message := buildOutgoingMessage(user, sendPayload, now)
	message.Source = "template"
	message.Tags = dedupeStrings([]string{"通知模板", "已发送", inviteRoleLabel(template.Role)})
	remoteResult, err := relayMailboxMessage(r.Context(), state.Config, user, message, sendPayload, SaveDraftPayload{}, "send")
	if err != nil {
		return domainError(err)
	}
	message = mergeRelayMessage(message, remoteResult.Message)
	acceptedAt := firstNonEmpty(remoteResult.AcceptedAt, message.SentAt, now.Format(time.RFC3339))
	providerMessageID := remoteResult.ProviderMessageID
	message.AcceptedAt = acceptedAt
	message.ProviderMessageID = providerMessageID
	message.DeliveryError = ""
	message.DeliveryStatus = firstNonEmpty(message.DeliveryStatus, "accepted")
	result, err := app.store.mutate(func(state *AppState) (any, error) {
		userIndex, ok := authenticatedUserIndex(token, state)
		if !ok {
			return nil, errUnauthorized
		}
		accountID := state.Users[userIndex].ID
		state.Messages[accountID] = upsertUserMessage(state.Messages[accountID], message)
		appendAuditLog(state, r, "user", "template.send", message.ID, fmt.Sprintf("%s · %d recipients", template.Role, len(recipients)))
		return envelope(response{
			"message":           message,
			"role":              template.Role,
			"recipientCount":    len(recipients),
			"acceptedAt":        acceptedAt,
			"providerMessageId": nullableString(providerMessageID),
		}), nil
	})
	if err != nil {
		return domainError(err)
	}
	return http.StatusOK, result
}

func (app *App) ingestInboundMail(r *http.Request) (int, any) {
	if !app.authorizedMailWebhook(r) {
		return http.StatusUnauthorized, response{"message": "邮件 Webhook 未授权或未配置 MAIL_WEBHOOK_TOKEN"}
	}
	var payload InboundMailPayload
	if err := decodeJSON(r, &payload); err != nil {
		return badRequest("invalid json body")
	}
	var autoReplyConfig AdminConfig
	var autoReplyUser UserRecord
	var autoReplySettings MailSettings
	var autoReplyInbound MailMessage
	shouldAutoReply := false
	result, err := app.store.mutate(func(state *AppState) (any, error) {
		accountIndex, err := resolveInboundAccountIndex(state.Users, payload)
		if err != nil {
			return nil, err
		}
		user := state.Users[accountIndex]
		message, err := buildInboundMailMessage(user, payload)
		if err != nil {
			return nil, err
		}
		attachments, err := app.persistAttachmentList(user.ID, message.Attachments, app.apiBasePath(r))
		if err != nil {
			return nil, err
		}
		message.Attachments = attachments
		message.HasAttachment = message.HasAttachment || len(attachments) > 0
		alreadyReplied := hasAutoReplyForMessage(state.Messages[user.ID], message.ID)
		state.Messages[user.ID] = upsertUserMessage(state.Messages[user.ID], message)
		settings := state.Settings[user.ID]
		if settings.DefaultSenderName == "" {
			settings = defaultSettings(user)
		}
		if !alreadyReplied && shouldSendAutoReply(user, settings, message, payload) {
			autoReplyConfig = state.Config
			autoReplyUser = user
			autoReplySettings = settings
			autoReplyInbound = message
			shouldAutoReply = true
		}
		appendAuditLog(state, r, "system", "mail.inbound", message.ID, "收信 Webhook 已写入")
		return envelope(response{"message": message}), nil
	})
	if err != nil {
		return domainError(err)
	}
	if shouldAutoReply {
		if _, err := app.sendAutoReply(r.Context(), autoReplyConfig, autoReplyUser, autoReplySettings, autoReplyInbound, r); err != nil {
			_, _ = app.store.mutate(func(state *AppState) (any, error) {
				appendAuditLog(state, r, "system", "mail.auto_reply.failed", autoReplyInbound.ID, err.Error())
				return nil, nil
			})
		}
	}
	return http.StatusOK, result
}

func (app *App) updateDeliveryStatus(r *http.Request) (int, any) {
	if !app.authorizedMailWebhook(r) {
		return http.StatusUnauthorized, response{"message": "邮件 Webhook 未授权或未配置 MAIL_WEBHOOK_TOKEN"}
	}
	var payload DeliveryStatusPayload
	if err := decodeJSON(r, &payload); err != nil {
		return badRequest("invalid json body")
	}
	result, err := app.store.mutate(func(state *AppState) (any, error) {
		message, ok := updateStoredDeliveryStatus(state, payload)
		if !ok {
			return nil, errors.New("投递状态对应的邮件不存在")
		}
		appendAuditLog(state, r, "system", "mail.delivery.update", message.ID, firstNonEmpty(message.DeliveryStatus, "unknown"))
		return envelope(response{"message": message}), nil
	})
	if err != nil {
		return domainError(err)
	}
	return http.StatusOK, result
}

func (app *App) updateMessageStar(r *http.Request) (int, any) {
	var patch MessageStarPatch
	if err := decodeJSON(r, &patch); err != nil {
		return badRequest("invalid json body")
	}
	messageID := strings.TrimSpace(r.PathValue("messageID"))
	if messageID == "" {
		return badRequest("邮件不存在")
	}
	state := app.store.snapshot()
	user, ok := authenticatedUser(r, state)
	if !ok {
		return unauthorized()
	}
	if err := ensureProvisionedMailbox(user); err != nil {
		return domainError(err)
	}
	message, ok := findMessageByID(state.Messages[user.ID], messageID)
	if !ok {
		return badRequest("邮件不存在")
	}
	nextStarred := !message.IsStarred
	if patch.Starred != nil {
		nextStarred = *patch.Starred
	}
	if _, err := syncMailboxMessageStar(r.Context(), state.Config, user, messageID, nextStarred); err != nil {
		return domainError(err)
	}
	token := sessionToken(r)
	now := nowISO()
	result, err := app.store.mutate(func(state *AppState) (any, error) {
		userIndex, ok := authenticatedUserIndex(token, state)
		if !ok {
			return nil, errUnauthorized
		}
		accountID := state.Users[userIndex].ID
		index := findMessageIndexByID(state.Messages[accountID], messageID)
		if index < 0 {
			return nil, errors.New("邮件不存在")
		}
		state.Messages[accountID][index].IsStarred = nextStarred
		state.Messages[accountID][index].SortAt = firstNonEmpty(state.Messages[accountID][index].SortAt, now)
		appendAuditLog(state, r, "user", "message.star.update", messageID, fmt.Sprintf("星标状态: %t", nextStarred))
		return envelope(response{"messageId": messageID, "starred": nextStarred, "updatedAt": now}), nil
	})
	if err != nil {
		return domainError(err)
	}
	return http.StatusOK, result
}

func (app *App) moveMessage(r *http.Request) (int, any) {
	var payload MessageMovePayload
	if err := decodeJSON(r, &payload); err != nil {
		return badRequest("invalid json body")
	}
	messageID := strings.TrimSpace(r.PathValue("messageID"))
	targetFolder := normalizeMessageFolder(payload.TargetFolder, "")
	if messageID == "" {
		return badRequest("邮件不存在")
	}
	if targetFolder == "" || targetFolder == "starred" {
		return badRequest("目标文件夹无效")
	}
	state := app.store.snapshot()
	user, ok := authenticatedUser(r, state)
	if !ok {
		return unauthorized()
	}
	if err := ensureProvisionedMailbox(user); err != nil {
		return domainError(err)
	}
	message, ok := findMessageByID(state.Messages[user.ID], messageID)
	if !ok {
		return badRequest("邮件不存在")
	}
	previousFolder := message.Folder
	if _, err := syncMailboxMessageMove(r.Context(), state.Config, user, messageID, previousFolder, targetFolder); err != nil {
		return domainError(err)
	}
	token := sessionToken(r)
	now := nowISO()
	result, err := app.store.mutate(func(state *AppState) (any, error) {
		userIndex, ok := authenticatedUserIndex(token, state)
		if !ok {
			return nil, errUnauthorized
		}
		accountID := state.Users[userIndex].ID
		index := findMessageIndexByID(state.Messages[accountID], messageID)
		if index < 0 {
			return nil, errors.New("邮件不存在")
		}
		state.Messages[accountID][index].PreviousFolder = previousFolder
		state.Messages[accountID][index].Folder = targetFolder
		appendAuditLog(state, r, "user", "message.move", messageID, previousFolder+" -> "+targetFolder)
		return envelope(response{"messageId": messageID, "previousFolder": previousFolder, "folder": targetFolder, "movedAt": now}), nil
	})
	if err != nil {
		return domainError(err)
	}
	return http.StatusOK, result
}

func (app *App) updateMessageReadState(r *http.Request) (int, any) {
	var patch MessageReadPatch
	if err := decodeJSON(r, &patch); err != nil {
		return badRequest("invalid json body")
	}
	messageID := strings.TrimSpace(r.PathValue("messageID"))
	if messageID == "" {
		return badRequest("邮件不存在")
	}
	state := app.store.snapshot()
	user, ok := authenticatedUser(r, state)
	if !ok {
		return unauthorized()
	}
	if err := ensureProvisionedMailbox(user); err != nil {
		return domainError(err)
	}
	message, ok := findMessageByID(state.Messages[user.ID], messageID)
	if !ok {
		return badRequest("邮件不存在")
	}
	nextUnread := false
	if patch.IsUnread != nil {
		nextUnread = *patch.IsUnread
	} else if patch.Read != nil {
		nextUnread = !*patch.Read
	}
	if _, err := syncMailboxMessageRead(r.Context(), state.Config, user, messageID, nextUnread); err != nil {
		return domainError(err)
	}
	token := sessionToken(r)
	now := nowISO()
	result, err := app.store.mutate(func(state *AppState) (any, error) {
		userIndex, ok := authenticatedUserIndex(token, state)
		if !ok {
			return nil, errUnauthorized
		}
		accountID := state.Users[userIndex].ID
		index := findMessageIndexByID(state.Messages[accountID], messageID)
		if index < 0 {
			return nil, errors.New("邮件不存在")
		}
		state.Messages[accountID][index].IsUnread = nextUnread
		state.Messages[accountID][index].SortAt = firstNonEmpty(state.Messages[accountID][index].SortAt, message.SortAt, now)
		appendAuditLog(state, r, "user", "message.read.update", messageID, fmt.Sprintf("未读状态: %t", nextUnread))
		return envelope(response{"messageId": messageID, "isUnread": nextUnread, "read": !nextUnread, "updatedAt": now}), nil
	})
	if err != nil {
		return domainError(err)
	}
	return http.StatusOK, result
}

func (app *App) getSettings(r *http.Request) (int, any) {
	state := app.store.snapshot()
	user, ok := authenticatedUser(r, state)
	if !ok {
		return unauthorized()
	}
	if err := ensureProvisionedMailbox(user); err != nil {
		return domainError(err)
	}
	settings := state.Settings[user.ID]
	if settings.DefaultSenderName == "" {
		settings = defaultSettings(user)
	}
	return http.StatusOK, envelope(response{"settings": settings})
}

func (app *App) updateSettings(r *http.Request) (int, any) {
	var patch mailSettingsPatch
	if err := decodeJSON(r, &patch); err != nil {
		return badRequest("invalid json body")
	}
	token := sessionToken(r)
	result, err := app.store.mutate(func(state *AppState) (any, error) {
		userIndex, ok := authenticatedUserIndex(token, state)
		if !ok {
			return nil, errUnauthorized
		}
		user := state.Users[userIndex]
		if err := ensureProvisionedMailbox(user); err != nil {
			return nil, err
		}
		current := state.Settings[user.ID]
		if current.DefaultSenderName == "" {
			current = defaultSettings(user)
		}
		if patch.DefaultSenderName != nil {
			current.DefaultSenderName = firstNonEmpty(strings.TrimSpace(*patch.DefaultSenderName), firstNonEmpty(user.DisplayName, user.Email, "InfiniteMail 用户"))
		}
		if patch.Signature != nil {
			current.Signature = strings.TrimSpace(*patch.Signature)
		}
		if patch.AutoReplyEnabled != nil {
			current.AutoReplyEnabled = *patch.AutoReplyEnabled
		}
		if patch.AutoReplyMessage != nil {
			current.AutoReplyMessage = strings.TrimSpace(*patch.AutoReplyMessage)
		}
		if strings.TrimSpace(current.AutoReplyMessage) == "" {
			current.AutoReplyMessage = defaultSettings(user).AutoReplyMessage
		}
		current.UpdatedAt = nowISO()
		state.Settings[user.ID] = current
		appendAuditLog(state, r, "user", "mail.settings.update", user.Email, "用户更新邮箱设置")
		return envelope(response{"settings": current}), nil
	})
	if err != nil {
		if errors.Is(err, errUnauthorized) {
			return unauthorized()
		}
		return domainError(err)
	}
	return http.StatusOK, result
}

func applyMailboxPatch(config *MailboxConfig, patch mailboxPatch) {
	previousDomain := config.Domain
	if patch.Domain != nil {
		config.Domain = normalizeDomain(*patch.Domain)
	}
	if patch.PrefixPolicyEnabled != nil {
		config.PrefixPolicyEnabled = *patch.PrefixPolicyEnabled
	}
	if patch.AllowedPrefixes != nil {
		config.AllowedPrefixes = normalizeAllowedPrefixes(patch.AllowedPrefixes)
	}
	if patch.DefaultPrefix != nil {
		config.DefaultPrefix = normalizePrefix(*patch.DefaultPrefix)
	}
	config.AllowedPrefixes = normalizeAllowedPrefixes(config.AllowedPrefixes)
	config.DefaultPrefix = resolveDefaultPrefix(*config)
	if !strings.EqualFold(previousDomain, config.Domain) {
		config.DNS = defaultDNSCheck(config.Domain)
	} else {
		config.DNS = normalizeDNSCheck(config.Domain, config.DNS)
	}
	if patch.Server != nil {
		applyMailServerPatch(&config.Server, *patch.Server)
	}
	config.Server = normalizeMailServerConfig(config.Server)
}

func applyAuthPatch(config *AuthConfig, patch authPatch) {
	methodsPatched := false
	if patch.OAuthEnabled != nil {
		config.OAuthEnabled = *patch.OAuthEnabled
	}
	if patch.OAuthProviderName != nil {
		config.OAuthProviderName = firstNonEmpty(strings.TrimSpace(*patch.OAuthProviderName), config.OAuthProviderName, "公司账号")
	}
	if patch.OAuthClientID != nil {
		config.OAuthClientID = strings.TrimSpace(*patch.OAuthClientID)
	}
	if patch.OAuthClientSecret != nil && strings.TrimSpace(*patch.OAuthClientSecret) != "" {
		config.OAuthClientSecret = strings.TrimSpace(*patch.OAuthClientSecret)
		config.OAuthClientSecretSet = true
		config.OAuthClientSecretMasked = maskSecret(config.OAuthClientSecret)
	}
	if patch.OAuthAuthorizeURL != nil {
		config.OAuthAuthorizeURL = strings.TrimSpace(*patch.OAuthAuthorizeURL)
	}
	if patch.OAuthTokenURL != nil {
		config.OAuthTokenURL = strings.TrimSpace(*patch.OAuthTokenURL)
	}
	if patch.OAuthUserInfoURL != nil {
		config.OAuthUserInfoURL = strings.TrimSpace(*patch.OAuthUserInfoURL)
	}
	if patch.OAuthRedirectURL != nil {
		config.OAuthRedirectURL = strings.TrimSpace(*patch.OAuthRedirectURL)
	}
	if patch.OAuthScopes != nil {
		config.OAuthScopes = normalizeOAuthScopes(patch.OAuthScopes)
	}
	if patch.OAuthSubjectField != nil {
		config.OAuthSubjectField = strings.TrimSpace(*patch.OAuthSubjectField)
	}
	if patch.OAuthPhoneField != nil {
		config.OAuthPhoneField = strings.TrimSpace(*patch.OAuthPhoneField)
	}
	if patch.OAuthEmailField != nil {
		config.OAuthEmailField = strings.TrimSpace(*patch.OAuthEmailField)
	}
	if patch.OAuthNameField != nil {
		config.OAuthNameField = strings.TrimSpace(*patch.OAuthNameField)
	}
	if patch.PasswordLoginEnabled != nil {
		config.PasswordLoginEnabled = *patch.PasswordLoginEnabled
	}
	if patch.PhoneLoginEnabled != nil {
		config.PhoneLoginEnabled = *patch.PhoneLoginEnabled
		methodsPatched = true
	}
	if patch.EmailLoginEnabled != nil {
		config.EmailLoginEnabled = *patch.EmailLoginEnabled
		methodsPatched = true
	}
	if methodsPatched && !config.PhoneLoginEnabled && !config.EmailLoginEnabled {
		config.PasswordLoginEnabled = false
	}
	if patch.RegistrationEnabled != nil {
		config.RegistrationEnabled = *patch.RegistrationEnabled
	}
	if patch.InviteRequired != nil {
		config.InviteRequired = *patch.InviteRequired
	}
	if patch.LoginLandingMode != nil {
		config.LoginLandingMode = strings.TrimSpace(*patch.LoginLandingMode)
	}
	*config = normalizeAuthConfig(*config)
}

func applySMSPatch(config *SMSConfig, patch smsPatch) {
	if patch.Provider != nil {
		config.Provider = firstNonEmpty(strings.TrimSpace(*patch.Provider), "aliyun")
	}
	if patch.AliyunEnabled != nil {
		config.AliyunEnabled = *patch.AliyunEnabled
	}
	if patch.AccessKeyID != nil {
		config.AccessKeyID = strings.TrimSpace(*patch.AccessKeyID)
	}
	if patch.AccessKeySecret != nil && strings.TrimSpace(*patch.AccessKeySecret) != "" {
		config.AccessKeySecret = strings.TrimSpace(*patch.AccessKeySecret)
		config.AccessKeySecretSet = true
		config.AccessKeySecretMasked = maskSecret(config.AccessKeySecret)
	}
	if patch.SignName != nil {
		config.SignName = strings.TrimSpace(*patch.SignName)
	}
	if patch.TemplateCode != nil {
		config.TemplateCode = strings.TrimSpace(*patch.TemplateCode)
	}
	if patch.CodeTTLMinutes != nil && *patch.CodeTTLMinutes > 0 {
		config.CodeTTLMinutes = *patch.CodeTTLMinutes
	}
}

func applyOpsPatch(config *OpsConfig, patch opsPatch) {
	if patch.AutoRunEnabled != nil {
		config.AutoRunEnabled = *patch.AutoRunEnabled
	}
	if patch.IntervalMinutes != nil {
		config.IntervalMinutes = *patch.IntervalMinutes
	}
	config.UpdatedAt = nowISO()
	*config = normalizeOpsConfig(*config)
}

func applyAdminSecurityPatch(config *AdminSecurityConfig, patch adminSecurityPatch) error {
	if patch.Username != nil {
		username := strings.TrimSpace(*patch.Username)
		if username == "" {
			return errors.New("管理员账号不能为空")
		}
		config.Username = username
	}
	now := nowISO()
	if patch.NewPassword != nil {
		password := strings.TrimSpace(*patch.NewPassword)
		if password != "" {
			if len(password) < 8 {
				return errors.New("管理员密码至少 8 位")
			}
			hash, err := hashPassword(password)
			if err != nil {
				return err
			}
			config.PasswordHash = hash
			config.PasswordSet = true
			config.PasswordUpdatedAt = now
		}
	}
	if patch.APIToken != nil {
		token := strings.TrimSpace(*patch.APIToken)
		if token != "" {
			if len(token) < 16 {
				return errors.New("后台 API Token 至少 16 位")
			}
			config.APITokenHash = hashAdminAPIToken(token)
			config.APITokenMasked = maskSecret(token)
			config.APITokenSet = true
			config.APITokenUpdatedAt = now
		}
	}
	if patch.ClearAPIToken != nil && *patch.ClearAPIToken {
		config.APITokenHash = ""
		config.APITokenMasked = ""
		config.APITokenSet = false
		config.APITokenUpdatedAt = now
	}
	config.UpdatedAt = now
	*config = normalizeAdminSecurityConfig(*config)
	return nil
}

func applyMailServerPatch(config *MailServerConfig, patch mailServerPatch) {
	if patch.Provider != nil {
		config.Provider = firstNonEmpty(strings.TrimSpace(*patch.Provider), "stalwart")
	}
	if patch.Enabled != nil {
		config.Enabled = *patch.Enabled
	}
	if patch.StrictDataPlane != nil {
		config.StrictDataPlane = *patch.StrictDataPlane
	}
	if patch.BaseURL != nil {
		config.BaseURL = strings.TrimRight(strings.TrimSpace(*patch.BaseURL), "/")
		config.Status = "unknown"
		config.LastError = ""
	}
	if patch.ProvisionPath != nil {
		config.ProvisionPath = normalizeProvisionPath(*patch.ProvisionPath)
	}
	if patch.LifecyclePath != nil {
		config.LifecyclePath = normalizeProvisionPath(*patch.LifecyclePath)
	}
	if patch.MessageListPath != nil {
		config.MessageListPath = normalizeProvisionPath(*patch.MessageListPath)
	}
	if patch.MessageDetailPath != nil {
		config.MessageDetailPath = normalizeProvisionPath(*patch.MessageDetailPath)
	}
	if patch.MessageSendPath != nil {
		config.MessageSendPath = normalizeProvisionPath(*patch.MessageSendPath)
	}
	if patch.DraftPath != nil {
		config.DraftPath = normalizeProvisionPath(*patch.DraftPath)
	}
	if patch.MessageStarPath != nil {
		config.MessageStarPath = normalizeProvisionPath(*patch.MessageStarPath)
	}
	if patch.MessageMovePath != nil {
		config.MessageMovePath = normalizeProvisionPath(*patch.MessageMovePath)
	}
	if patch.MessageReadPath != nil {
		config.MessageReadPath = normalizeProvisionPath(*patch.MessageReadPath)
	}
	if patch.AdminToken != nil && strings.TrimSpace(*patch.AdminToken) != "" {
		config.AdminToken = strings.TrimSpace(*patch.AdminToken)
		config.AdminTokenSet = true
		config.AdminTokenMasked = maskSecret(*patch.AdminToken)
	}
	if patch.SMTPEnabled != nil {
		config.SMTPEnabled = *patch.SMTPEnabled
	}
	if patch.SMTPHost != nil {
		config.SMTPHost = strings.TrimSpace(*patch.SMTPHost)
		config.Status = "unknown"
		config.LastError = ""
	}
	if patch.SMTPPort != nil && *patch.SMTPPort > 0 && *patch.SMTPPort <= 65535 {
		config.SMTPPort = *patch.SMTPPort
	}
	if patch.SMTPUsername != nil {
		config.SMTPUsername = strings.TrimSpace(*patch.SMTPUsername)
	}
	if patch.SMTPPassword != nil && strings.TrimSpace(*patch.SMTPPassword) != "" {
		config.SMTPPassword = strings.TrimSpace(*patch.SMTPPassword)
		config.SMTPPasswordSet = true
		config.SMTPPasswordMasked = maskSecret(*patch.SMTPPassword)
	}
	if patch.SMTPTLSMode != nil {
		config.SMTPTLSMode = strings.TrimSpace(*patch.SMTPTLSMode)
	}
	if patch.IMAPEnabled != nil {
		config.IMAPEnabled = *patch.IMAPEnabled
	}
	if patch.IMAPHost != nil {
		config.IMAPHost = strings.TrimSpace(*patch.IMAPHost)
		config.Status = "unknown"
		config.LastError = ""
	}
	if patch.IMAPPort != nil && *patch.IMAPPort > 0 && *patch.IMAPPort <= 65535 {
		config.IMAPPort = *patch.IMAPPort
	}
	if patch.IMAPUsername != nil {
		config.IMAPUsername = strings.TrimSpace(*patch.IMAPUsername)
	}
	if patch.IMAPPassword != nil && strings.TrimSpace(*patch.IMAPPassword) != "" {
		config.IMAPPassword = strings.TrimSpace(*patch.IMAPPassword)
		config.IMAPPasswordSet = true
		config.IMAPPasswordMasked = maskSecret(*patch.IMAPPassword)
	}
	if patch.IMAPTLSMode != nil {
		config.IMAPTLSMode = strings.TrimSpace(*patch.IMAPTLSMode)
	}
	*config = normalizeMailServerConfig(*config)
}

func decodeJSON(r *http.Request, target any) error {
	defer r.Body.Close()
	return json.NewDecoder(r.Body).Decode(target)
}

func ensureSMSCode(logs []SMSLogRecord, phone string, code string, purpose string) error {
	code = strings.TrimSpace(code)
	for _, log := range logs {
		if log.Phone != phone || log.Purpose != purpose {
			continue
		}
		expiresAt, err := time.Parse(time.RFC3339, log.ExpiresAt)
		if err == nil && time.Now().After(expiresAt) {
			return errors.New("验证码不正确或已过期")
		}
		if verifySMSCode(log, code) {
			return nil
		}
		return errors.New("验证码不正确或已过期")
	}
	return errors.New("验证码不正确或已过期")
}

func hashSMSCode(phone string, purpose string, code string) (string, error) {
	return hashPassword(smsCodeCredential(phone, purpose, code))
}

func verifySMSCode(log SMSLogRecord, code string) bool {
	code = strings.TrimSpace(code)
	if code == "" {
		return false
	}
	if strings.TrimSpace(log.CodeHash) != "" {
		return verifyPassword(log.CodeHash, smsCodeCredential(log.Phone, log.Purpose, code))
	}
	return constantTimeStringEqual(strings.TrimSpace(log.Code), code)
}

func smsCodeCredential(phone string, purpose string, code string) string {
	return strings.Join([]string{
		"sms-v1",
		normalizePhone(phone),
		strings.TrimSpace(purpose),
		strings.TrimSpace(code),
	}, ":")
}

func maskSMSCode(code string) string {
	code = strings.TrimSpace(code)
	if len(code) <= 2 {
		return ""
	}
	return code[:2] + strings.Repeat("*", len(code)-2)
}

func ensureSMSCanSend(logs []SMSLogRecord, phone string, purpose string, window time.Duration) error {
	for _, log := range logs {
		if log.Phone != phone || log.Purpose != purpose {
			continue
		}
		createdAt, err := time.Parse(time.RFC3339, log.CreatedAt)
		if err == nil && time.Since(createdAt) < window {
			return errors.New("验证码发送太频繁，请稍后再试")
		}
		return nil
	}
	return nil
}

func defaultMailServerConfig() MailServerConfig {
	return MailServerConfig{
		Provider:    "stalwart",
		Enabled:     false,
		SMTPPort:    25,
		SMTPTLSMode: "auto",
		IMAPPort:    993,
		IMAPTLSMode: "tls",
		Status:      "not_configured",
	}
}

func normalizeAdminSecurityConfig(config AdminSecurityConfig) AdminSecurityConfig {
	config.Username = firstNonEmpty(strings.TrimSpace(config.Username), "admin")
	config.PasswordHash = strings.TrimSpace(config.PasswordHash)
	config.PasswordSet = config.PasswordHash != ""
	config.APITokenHash = strings.TrimSpace(config.APITokenHash)
	config.APITokenSet = config.APITokenHash != ""
	config.APITokenMasked = strings.TrimSpace(config.APITokenMasked)
	if !config.APITokenSet {
		config.APITokenMasked = ""
	}
	return config
}

func normalizeAuthConfig(config AuthConfig) AuthConfig {
	config.OAuthProviderName = firstNonEmpty(strings.TrimSpace(config.OAuthProviderName), "悦享账号")
	if config.OAuthProviderName == legacyYuexiangFoodName() {
		config.OAuthProviderName = "悦享账号"
	}
	config.OAuthClientID = strings.TrimSpace(config.OAuthClientID)
	config.OAuthClientSecret = strings.TrimSpace(config.OAuthClientSecret)
	if config.OAuthClientSecret != "" {
		config.OAuthClientSecretSet = true
		config.OAuthClientSecretMasked = maskSecret(config.OAuthClientSecret)
	}
	config.OAuthAuthorizeURL = strings.TrimSpace(config.OAuthAuthorizeURL)
	config.OAuthTokenURL = strings.TrimSpace(config.OAuthTokenURL)
	config.OAuthUserInfoURL = strings.TrimSpace(config.OAuthUserInfoURL)
	config.OAuthRedirectURL = strings.TrimSpace(config.OAuthRedirectURL)
	config.OAuthScopes = normalizeOAuthScopes(config.OAuthScopes)
	config.OAuthSubjectField = firstNonEmpty(config.OAuthSubjectField, "sub")
	config.OAuthPhoneField = firstNonEmpty(config.OAuthPhoneField, "phone")
	config.OAuthEmailField = firstNonEmpty(config.OAuthEmailField, "email")
	config.OAuthNameField = firstNonEmpty(config.OAuthNameField, "name")
	if config.PasswordLoginEnabled && !config.PhoneLoginEnabled && !config.EmailLoginEnabled {
		config.PhoneLoginEnabled = true
	}
	if config.PhoneLoginEnabled || config.EmailLoginEnabled {
		config.PasswordLoginEnabled = true
	}
	switch strings.ToLower(strings.TrimSpace(config.LoginLandingMode)) {
	case "account", "password":
		config.LoginLandingMode = "account"
	case "oauth":
		config.LoginLandingMode = "oauth"
	default:
		if config.OAuthEnabled {
			config.LoginLandingMode = "oauth"
		} else {
			config.LoginLandingMode = "account"
		}
	}
	if config.LoginLandingMode == "oauth" && !config.OAuthEnabled && config.PasswordLoginEnabled {
		config.LoginLandingMode = "account"
	}
	if config.LoginLandingMode == "account" && !config.PasswordLoginEnabled && config.OAuthEnabled {
		config.LoginLandingMode = "oauth"
	}
	return config
}

func normalizeOAuthScopes(values []string) []string {
	seen := map[string]bool{}
	scopes := []string{}
	for _, value := range values {
		for _, part := range strings.FieldsFunc(value, func(r rune) bool {
			return r == ',' || r == '，' || r == ' ' || r == '\n' || r == '\t'
		}) {
			scope := strings.TrimSpace(part)
			if scope == "" || seen[scope] {
				continue
			}
			seen[scope] = true
			scopes = append(scopes, scope)
		}
	}
	if len(scopes) == 0 {
		return []string{"openid", "profile", "email", "phone"}
	}
	return scopes
}

func normalizeOpsConfig(config OpsConfig) OpsConfig {
	if config.IntervalMinutes <= 0 {
		config.IntervalMinutes = 5
	}
	if config.IntervalMinutes > 1440 {
		config.IntervalMinutes = 1440
	}
	switch strings.ToLower(strings.TrimSpace(config.LastRunStatus)) {
	case "success", "warning", "failed", "running":
		config.LastRunStatus = strings.ToLower(strings.TrimSpace(config.LastRunStatus))
	case "idle":
		config.LastRunStatus = "idle"
	default:
		if strings.TrimSpace(config.LastRunAt) == "" {
			config.LastRunStatus = "idle"
		} else {
			config.LastRunStatus = "success"
		}
	}
	config.LastRunAt = strings.TrimSpace(config.LastRunAt)
	config.LastRunMessage = strings.TrimSpace(config.LastRunMessage)
	config.UpdatedAt = strings.TrimSpace(config.UpdatedAt)
	return config
}

func normalizeProvisionJob(job ProvisionJob) ProvisionJob {
	job.ID = firstNonEmpty(strings.TrimSpace(job.ID), nextID("prov"))
	job.AccountID = strings.TrimSpace(job.AccountID)
	job.Email = strings.TrimSpace(job.Email)
	job.Status = normalizeProvisionJobStatus(job.Status)
	if job.Attempts < 0 {
		job.Attempts = 0
	}
	job.LastError = strings.TrimSpace(job.LastError)
	if strings.TrimSpace(job.CreatedAt) == "" {
		job.CreatedAt = nowISO()
	}
	if strings.TrimSpace(job.UpdatedAt) == "" {
		job.UpdatedAt = job.CreatedAt
	}
	if job.Status == "succeeded" {
		job.LastError = ""
		if strings.TrimSpace(job.CompletedAt) == "" {
			job.CompletedAt = job.UpdatedAt
		}
	}
	return job
}

func normalizeProvisionJobStatus(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "queued", "running", "succeeded", "failed", "blocked":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return "queued"
	}
}

func queueProvisionJob(state *AppState, user UserRecord, now string) ProvisionJob {
	jobStatus := "queued"
	lastError := ""
	server := normalizeMailServerConfig(state.Config.Mailbox.Server)
	if !server.Enabled || server.BaseURL == "" {
		jobStatus = "blocked"
		lastError = "邮件服务未配置"
	} else if server.Status != "online" {
		jobStatus = "failed"
		lastError = firstNonEmpty(server.LastError, "邮件服务未连通")
	}
	for index := range state.Config.ProvisionJobs {
		job := &state.Config.ProvisionJobs[index]
		if job.AccountID != user.ID || job.Status == "succeeded" {
			continue
		}
		job.Email = user.Email
		job.Status = jobStatus
		job.LastError = lastError
		job.NextRunAt = now
		job.UpdatedAt = now
		job.CompletedAt = ""
		*job = normalizeProvisionJob(*job)
		return *job
	}
	job := normalizeProvisionJob(ProvisionJob{
		ID:        nextID("prov"),
		AccountID: user.ID,
		Email:     user.Email,
		Status:    jobStatus,
		LastError: lastError,
		NextRunAt: now,
		CreatedAt: now,
		UpdatedAt: now,
	})
	state.Config.ProvisionJobs = append([]ProvisionJob{job}, state.Config.ProvisionJobs...)
	if len(state.Config.ProvisionJobs) > 300 {
		state.Config.ProvisionJobs = state.Config.ProvisionJobs[:300]
	}
	return job
}

func runOperationalTasksState(ctx context.Context, state *AppState, now time.Time) OperationalRunSummary {
	ensureProvisionJobsForAccounts(state, now.Format(time.RFC3339))
	provisioning := processProvisionJobs(ctx, state, 25, now)
	queued, failed, completed := provisionJobCounts(state.Config.ProvisionJobs)
	return OperationalRunSummary{
		CheckedAt:              now.Format(time.RFC3339),
		Provisioning:           provisioning,
		QueuedProvisionJobs:    queued,
		FailedProvisionJobs:    failed,
		CompletedProvisionJobs: completed,
	}
}

func applyOperationalRunMetadata(config *AdminConfig, summary OperationalRunSummary) {
	config.Ops = normalizeOpsConfig(config.Ops)
	config.Ops.LastRunAt = summary.CheckedAt
	config.Ops.LastRunStatus = stateSummaryStatus(summary)
	config.Ops.LastRunMessage = summaryMessage(summary)
	config.Ops.UpdatedAt = summary.CheckedAt
}

func opsAutoRunDue(config OpsConfig, now time.Time) bool {
	config = normalizeOpsConfig(config)
	if !config.AutoRunEnabled {
		return false
	}
	if strings.TrimSpace(config.LastRunAt) == "" {
		return true
	}
	lastRunAt, err := time.Parse(time.RFC3339, config.LastRunAt)
	if err != nil {
		return true
	}
	return now.Sub(lastRunAt) >= time.Duration(config.IntervalMinutes)*time.Minute
}

func stateSummaryStatus(summary OperationalRunSummary) string {
	if summary.Provisioning.Failed > 0 || summary.FailedProvisionJobs > 0 {
		return "warning"
	}
	return "success"
}

func summaryMessage(summary OperationalRunSummary) string {
	provisioningMessage := strings.TrimSpace(summary.Provisioning.Message)
	if provisioningMessage == "" {
		return "公司邮箱任务正常"
	}
	return provisioningMessage
}

func provisionMailboxAccount(ctx context.Context, config AdminConfig, user UserRecord, job ProvisionJob) (MailboxProvisionResult, error) {
	server := normalizeMailServerConfig(config.Mailbox.Server)
	localPart, domain := splitEmailAddress(user.Email)
	if localPart == "" || domain == "" {
		return MailboxProvisionResult{}, errors.New("邮箱地址无效，无法开通")
	}
	payload := MailboxProvisionPayload{
		ProvisionJobID: job.ID,
		AccountID:      user.ID,
		Email:          user.Email,
		LocalPart:      localPart,
		Domain:         domain,
		DisplayName:    user.DisplayName,
		Phone:          user.Phone,
		Source:         user.Source,
		Password:       randomMailboxPassword(),
		Metadata: map[string]any{
			"provider": "infinitemail",
			"jobId":    job.ID,
		},
	}
	endpoint := mailServerProvisionEndpoint(server)
	if endpoint == "" {
		if stalwartJMAPReady(server) {
			result, err := provisionStalwartMailboxAccount(ctx, server, payload)
			if err != nil {
				return MailboxProvisionResult{}, err
			}
			if strings.ToLower(strings.TrimSpace(result.Status)) != "exists" {
				result.MailboxUsername = firstNonEmpty(result.MailboxUsername, payload.Email)
				result.MailboxPassword = payload.Password
			}
			return result, nil
		}
		return MailboxProvisionResult{}, missingMailDataPlaneEndpointError("邮箱开通接口")
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return MailboxProvisionResult{}, fmt.Errorf("开通请求编码失败: %w", err)
	}
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(raw))
	if err != nil {
		return MailboxProvisionResult{}, errors.New("邮箱开通接口地址无效")
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Idempotency-Key", job.ID)
	applyMailServerAuthHeaders(req, server)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return MailboxProvisionResult{}, fmt.Errorf("邮箱开通接口调用失败: %w", err)
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(res.Body, 4096))
	if res.StatusCode == http.StatusConflict {
		result := parseMailboxProvisionResult(body)
		result.ExternalID = firstNonEmpty(result.ExternalID, user.MailboxExternalID, "existing_"+user.ID)
		result.Status = firstNonEmpty(result.Status, "exists")
		result.Message = firstNonEmpty(result.Message, "邮箱账号已存在，按幂等成功处理")
		return result, nil
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		message := strings.TrimSpace(string(body))
		if message == "" {
			message = http.StatusText(res.StatusCode)
		}
		return MailboxProvisionResult{}, fmt.Errorf("邮箱开通接口返回 HTTP %d: %s", res.StatusCode, message)
	}
	result := parseMailboxProvisionResult(body)
	result.ExternalID = firstNonEmpty(result.ExternalID, user.MailboxExternalID, "remote_"+user.ID)
	result.Status = firstNonEmpty(result.Status, "provisioned")
	result.MailboxUsername = firstNonEmpty(result.MailboxUsername, payload.Email)
	result.MailboxPassword = payload.Password
	return result, nil
}

func mailServerProvisionEndpoint(config MailServerConfig) string {
	config = normalizeMailServerConfig(config)
	return mailServerEndpoint(config, config.ProvisionPath)
}

func mailServerLifecycleEndpoint(config MailServerConfig) string {
	config = normalizeMailServerConfig(config)
	return mailServerEndpoint(config, config.LifecyclePath)
}

func mailServerMessageListEndpoint(config MailServerConfig) string {
	config = normalizeMailServerConfig(config)
	if !config.Enabled {
		return ""
	}
	return mailServerEndpoint(config, messageDataPath(config, config.MessageListPath, "/api/v1/messages"))
}

func mailServerMessageDetailEndpoint(config MailServerConfig, messageID string) string {
	config = normalizeMailServerConfig(config)
	if !config.Enabled {
		return ""
	}
	path := messageDataPath(config, config.MessageDetailPath, "/api/v1/messages/{messageId}")
	if path == "" {
		return ""
	}
	return mailServerEndpoint(config, interpolateMessagePath(path, messageID))
}

func mailServerMessageSendEndpoint(config MailServerConfig) string {
	config = normalizeMailServerConfig(config)
	if !config.Enabled {
		return ""
	}
	return mailServerEndpoint(config, messageDataPath(config, config.MessageSendPath, "/api/v1/messages/send"))
}

func mailServerDraftEndpoint(config MailServerConfig) string {
	config = normalizeMailServerConfig(config)
	if !config.Enabled {
		return ""
	}
	return mailServerEndpoint(config, messageDataPath(config, config.DraftPath, "/api/v1/drafts"))
}

func mailServerMessageStarEndpoint(config MailServerConfig, messageID string) string {
	config = normalizeMailServerConfig(config)
	if !config.Enabled {
		return ""
	}
	path := messageDataPath(config, config.MessageStarPath, "/api/v1/messages/{messageId}/star")
	return mailServerEndpoint(config, interpolateMessagePath(path, messageID))
}

func mailServerMessageMoveEndpoint(config MailServerConfig, messageID string) string {
	config = normalizeMailServerConfig(config)
	if !config.Enabled {
		return ""
	}
	path := messageDataPath(config, config.MessageMovePath, "/api/v1/messages/{messageId}/move")
	return mailServerEndpoint(config, interpolateMessagePath(path, messageID))
}

func mailServerMessageReadEndpoint(config MailServerConfig, messageID string) string {
	config = normalizeMailServerConfig(config)
	if !config.Enabled {
		return ""
	}
	path := messageDataPath(config, config.MessageReadPath, "/api/v1/messages/{messageId}/read")
	return mailServerEndpoint(config, interpolateMessagePath(path, messageID))
}

func interpolateMessagePath(path string, messageID string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	escapedID := url.PathEscape(strings.TrimSpace(messageID))
	path = strings.ReplaceAll(path, "{messageId}", escapedID)
	path = strings.ReplaceAll(path, ":messageId", escapedID)
	if !strings.Contains(path, escapedID) {
		path = strings.TrimRight(path, "/") + "/" + escapedID
	}
	return path
}

func messageDataPath(config MailServerConfig, configured string, fallback string) string {
	configured = normalizeProvisionPath(configured)
	if configured != "" {
		return configured
	}
	return ""
}

func missingMailDataPlaneEndpointError(label string) error {
	return fmt.Errorf("需要配置真实%s，已禁止本地假成功", label)
}

func mailServerEndpoint(config MailServerConfig, path string) string {
	path = normalizeProvisionPath(path)
	if strings.TrimSpace(path) == "" {
		return ""
	}
	if strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") {
		return strings.TrimRight(path, "/")
	}
	if strings.TrimSpace(config.BaseURL) == "" {
		return ""
	}
	return strings.TrimRight(config.BaseURL, "/") + path
}

func applyMailServerAuthHeaders(req *http.Request, config MailServerConfig) {
	token := strings.TrimSpace(config.AdminToken)
	if token == "" {
		return
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-Admin-Token", token)
}

func parseMailboxProvisionResult(raw []byte) MailboxProvisionResult {
	if len(raw) == 0 {
		return MailboxProvisionResult{}
	}
	var result MailboxProvisionResult
	_ = json.Unmarshal(raw, &result)
	var generic map[string]any
	if err := json.Unmarshal(raw, &generic); err == nil {
		result.ExternalID = firstNonEmpty(
			result.ExternalID,
			stringFromAny(generic["externalId"]),
			stringFromAny(generic["externalID"]),
			stringFromAny(generic["principalId"]),
			stringFromAny(generic["principalID"]),
			stringFromAny(generic["id"]),
			stringFromAny(generic["accountId"]),
		)
		result.Status = firstNonEmpty(result.Status, stringFromAny(generic["status"]))
		result.Message = firstNonEmpty(result.Message, stringFromAny(generic["message"]))
		result.MailboxUsername = firstNonEmpty(
			result.MailboxUsername,
			stringFromAny(generic["mailboxUsername"]),
			stringFromAny(generic["username"]),
			stringFromAny(generic["login"]),
		)
	}
	return result
}

func stalwartJMAPReady(config MailServerConfig) bool {
	config = normalizeMailServerConfig(config)
	provider := strings.ToLower(strings.TrimSpace(config.Provider))
	return config.Enabled &&
		strings.Contains(provider, "stalwart") &&
		strings.TrimSpace(config.BaseURL) != "" &&
		strings.TrimSpace(config.AdminToken) != ""
}

type stalwartJMAPMethodResponse struct {
	Name      string
	Args      map[string]any
	ClientID  string
	RawMethod []json.RawMessage
}

func provisionStalwartMailboxAccount(ctx context.Context, server MailServerConfig, payload MailboxProvisionPayload) (MailboxProvisionResult, error) {
	if payload.LocalPart == "" || payload.Domain == "" || payload.Email == "" {
		return MailboxProvisionResult{}, errors.New("Stalwart 开通缺少邮箱地址")
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	domainID, err := ensureStalwartDomain(ctx, server, payload.Domain)
	if err != nil {
		return MailboxProvisionResult{}, err
	}
	accountID, err := findStalwartAccount(ctx, server, payload.LocalPart, domainID)
	if err != nil {
		return MailboxProvisionResult{}, err
	}
	if accountID != "" {
		return MailboxProvisionResult{
			ExternalID: accountID,
			Status:     "exists",
			Message:    "Stalwart 邮箱账号已存在，按幂等成功处理",
		}, nil
	}

	accountID, err = createStalwartAccount(ctx, server, domainID, payload)
	if err != nil {
		return MailboxProvisionResult{}, err
	}
	return MailboxProvisionResult{
		ExternalID: accountID,
		Status:     "provisioned",
		Message:    "Stalwart 邮箱账号已开通",
	}, nil
}

func ensureStalwartDomain(ctx context.Context, server MailServerConfig, domain string) (string, error) {
	domain = strings.ToLower(strings.TrimSpace(domain))
	if domain == "" {
		return "", errors.New("Stalwart 域名为空")
	}
	domainID, err := queryStalwartFirstID(ctx, server, "x:Domain/query", "domain-query", map[string]any{"name": domain})
	if err != nil {
		return "", fmt.Errorf("查询 Stalwart 域名失败: %w", err)
	}
	if domainID != "" {
		return domainID, nil
	}
	createdID, err := createStalwartObject(ctx, server, "x:Domain/set", "domain-create", "domain", map[string]any{
		"name":                  domain,
		"aliases":               map[string]any{},
		"certificateManagement": map[string]any{"@type": "Manual"},
		"dkimManagement":        map[string]any{"@type": "Automatic"},
		"dnsManagement":         map[string]any{"@type": "Manual"},
		"subAddressing":         map[string]any{"@type": "Enabled"},
	})
	if err != nil {
		return "", fmt.Errorf("创建 Stalwart 域名失败: %w", err)
	}
	return createdID, nil
}

func findStalwartAccount(ctx context.Context, server MailServerConfig, localPart string, domainID string) (string, error) {
	localPart = strings.TrimSpace(localPart)
	domainID = strings.TrimSpace(domainID)
	if localPart == "" || domainID == "" {
		return "", errors.New("Stalwart 账号查询缺少本地名或域名 ID")
	}
	accountID, err := queryStalwartFirstID(ctx, server, "x:Account/query", "account-query", map[string]any{
		"name":     localPart,
		"domainId": domainID,
	})
	if err != nil {
		return "", fmt.Errorf("查询 Stalwart 账号失败: %w", err)
	}
	return accountID, nil
}

func createStalwartAccount(ctx context.Context, server MailServerConfig, domainID string, payload MailboxProvisionPayload) (string, error) {
	account := map[string]any{
		"@type":            "User",
		"name":             payload.LocalPart,
		"domainId":         domainID,
		"credentials":      stalwartPasswordCredentials(payload.Password),
		"memberGroupIds":   map[string]any{},
		"roles":            map[string]any{"@type": "User"},
		"permissions":      map[string]any{"@type": "Inherit"},
		"quotas":           map[string]any{},
		"aliases":          map[string]any{},
		"encryptionAtRest": map[string]any{"@type": "Disabled"},
	}
	if displayName := strings.TrimSpace(payload.DisplayName); displayName != "" {
		account["description"] = displayName
	}
	accountID, err := createStalwartObject(ctx, server, "x:Account/set", "account-create", "account", account)
	if err != nil {
		return "", fmt.Errorf("创建 Stalwart 账号失败: %w", err)
	}
	return accountID, nil
}

func syncStalwartMailboxLifecycle(ctx context.Context, server MailServerConfig, payload MailboxLifecyclePayload) (MailboxLifecycleResult, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	accountID := strings.TrimSpace(payload.ExternalID)
	if accountID == "" {
		domainID, err := ensureStalwartDomain(ctx, server, payload.Domain)
		if err != nil {
			return MailboxLifecycleResult{}, err
		}
		foundID, err := findStalwartAccount(ctx, server, payload.LocalPart, domainID)
		if err != nil {
			return MailboxLifecycleResult{}, err
		}
		accountID = foundID
	}
	if accountID == "" {
		return MailboxLifecycleResult{}, errors.New("Stalwart 邮箱账号不存在，无法同步生命周期")
	}

	update := map[string]any{}
	status := "synced"
	message := "Stalwart 邮箱账号已同步"
	switch strings.ToLower(strings.TrimSpace(payload.Action)) {
	case "disable":
		update["permissions"] = map[string]any{
			"@type":               "Replace",
			"enabledPermissions":  map[string]any{},
			"disabledPermissions": []string{"authenticate"},
		}
		status = "disabled"
		message = "Stalwart 邮箱账号已禁用认证"
	case "enable":
		update["permissions"] = map[string]any{"@type": "Inherit"}
		status = "enabled"
		message = "Stalwart 邮箱账号已恢复认证"
	case "reset_password":
		if strings.TrimSpace(payload.Password) == "" {
			return MailboxLifecycleResult{}, errors.New("重置 Stalwart 邮箱密码缺少临时密码")
		}
		update["credentials"] = stalwartPasswordCredentials(payload.Password)
		status = "password_reset"
		message = "Stalwart 邮箱密码已重置"
	default:
		return MailboxLifecycleResult{}, fmt.Errorf("不支持的 Stalwart 生命周期动作：%s", payload.Action)
	}
	if err := updateStalwartAccount(ctx, server, accountID, update); err != nil {
		return MailboxLifecycleResult{}, err
	}
	return MailboxLifecycleResult{
		ExternalID: accountID,
		Status:     status,
		Message:    message,
	}, nil
}

func queryStalwartFirstID(ctx context.Context, server MailServerConfig, methodName string, clientID string, filter map[string]any) (string, error) {
	responses, err := callStalwartJMAP(ctx, server, []any{
		[]any{methodName, map[string]any{"filter": filter, "limit": 1}, clientID},
	})
	if err != nil {
		return "", err
	}
	response, ok := stalwartMethodResponse(responses, methodName, clientID)
	if !ok {
		return "", fmt.Errorf("Stalwart 未返回 %s 响应", methodName)
	}
	if err := stalwartMethodError(response); err != nil {
		return "", err
	}
	ids := anySliceFromAny(response.Args["ids"])
	if len(ids) == 0 {
		return "", nil
	}
	return stringFromAny(ids[0]), nil
}

func createStalwartObject(ctx context.Context, server MailServerConfig, methodName string, clientID string, createKey string, object map[string]any) (string, error) {
	responses, err := callStalwartJMAP(ctx, server, []any{
		[]any{methodName, map[string]any{"create": map[string]any{createKey: object}}, clientID},
	})
	if err != nil {
		return "", err
	}
	response, ok := stalwartMethodResponse(responses, methodName, clientID)
	if !ok {
		return "", fmt.Errorf("Stalwart 未返回 %s 响应", methodName)
	}
	if err := stalwartMethodError(response); err != nil {
		return "", err
	}
	if err := stalwartSetErrors(response.Args, "notCreated", createKey); err != nil {
		return "", err
	}
	created := mapFromAny(response.Args["created"])
	item := mapFromAny(created[createKey])
	id := firstNonEmpty(stringFromAny(item["id"]), stringFromAny(item["accountId"]))
	if id == "" {
		return "", fmt.Errorf("Stalwart %s 未返回对象 ID", createKey)
	}
	return id, nil
}

func updateStalwartAccount(ctx context.Context, server MailServerConfig, accountID string, update map[string]any) error {
	accountID = strings.TrimSpace(accountID)
	if accountID == "" {
		return errors.New("Stalwart 账号 ID 为空")
	}
	responses, err := callStalwartJMAP(ctx, server, []any{
		[]any{"x:Account/set", map[string]any{"update": map[string]any{accountID: update}}, "account-update"},
	})
	if err != nil {
		return err
	}
	response, ok := stalwartMethodResponse(responses, "x:Account/set", "account-update")
	if !ok {
		return errors.New("Stalwart 未返回账号更新响应")
	}
	if err := stalwartMethodError(response); err != nil {
		return err
	}
	return stalwartSetErrors(response.Args, "notUpdated", accountID)
}

func callStalwartJMAP(ctx context.Context, server MailServerConfig, methodCalls []any) ([]stalwartJMAPMethodResponse, error) {
	endpoint := stalwartJMAPEndpoint(server)
	if endpoint == "" {
		return nil, errors.New("Stalwart JMAP 管理接口地址未配置")
	}
	raw, err := json.Marshal(map[string]any{
		"using":       []string{"urn:ietf:params:jmap:core", "urn:stalwart:jmap"},
		"methodCalls": methodCalls,
	})
	if err != nil {
		return nil, fmt.Errorf("Stalwart JMAP 请求编码失败: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(raw))
	if err != nil {
		return nil, errors.New("Stalwart JMAP 管理接口地址无效")
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	applyMailServerAuthHeaders(req, server)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Stalwart JMAP 调用失败: %w", err)
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(res.Body, 128*1024))
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		message := strings.TrimSpace(string(body))
		if message == "" {
			message = http.StatusText(res.StatusCode)
		}
		return nil, fmt.Errorf("Stalwart JMAP 返回 HTTP %d: %s", res.StatusCode, message)
	}
	var parsed struct {
		MethodResponses []json.RawMessage `json:"methodResponses"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("Stalwart JMAP 响应解析失败: %w", err)
	}
	responses := make([]stalwartJMAPMethodResponse, 0, len(parsed.MethodResponses))
	for _, rawResponse := range parsed.MethodResponses {
		var tuple []json.RawMessage
		if err := json.Unmarshal(rawResponse, &tuple); err != nil || len(tuple) < 2 {
			continue
		}
		var name string
		_ = json.Unmarshal(tuple[0], &name)
		var args map[string]any
		_ = json.Unmarshal(tuple[1], &args)
		clientID := ""
		if len(tuple) > 2 {
			_ = json.Unmarshal(tuple[2], &clientID)
		}
		responses = append(responses, stalwartJMAPMethodResponse{
			Name:      name,
			Args:      args,
			ClientID:  clientID,
			RawMethod: tuple,
		})
	}
	if len(responses) == 0 {
		return nil, errors.New("Stalwart JMAP 响应为空")
	}
	return responses, nil
}

func stalwartJMAPEndpoint(config MailServerConfig) string {
	config = normalizeMailServerConfig(config)
	base := strings.TrimRight(strings.TrimSpace(config.BaseURL), "/")
	if base == "" {
		return ""
	}
	parsed, err := url.Parse(base)
	if err != nil {
		return ""
	}
	trimmedPath := strings.TrimRight(parsed.Path, "/")
	if trimmedPath == "" {
		parsed.Path = "/api"
		return parsed.String()
	}
	if path.Base(trimmedPath) == "api" {
		parsed.Path = trimmedPath
		return parsed.String()
	}
	parsed.Path = trimmedPath + "/api"
	return parsed.String()
}

func stalwartPasswordCredentials(password string) map[string]any {
	return map[string]any{
		"password": map[string]any{
			"@type":       "Password",
			"secret":      password,
			"permissions": map[string]any{"@type": "Inherit"},
		},
	}
}

func stalwartMethodResponse(responses []stalwartJMAPMethodResponse, methodName string, clientID string) (stalwartJMAPMethodResponse, bool) {
	for _, response := range responses {
		if response.Name == methodName && (clientID == "" || response.ClientID == clientID) {
			return response, true
		}
	}
	for _, response := range responses {
		if response.Name == "error" && (clientID == "" || response.ClientID == clientID) {
			return response, true
		}
	}
	return stalwartJMAPMethodResponse{}, false
}

func stalwartMethodError(response stalwartJMAPMethodResponse) error {
	if response.Name != "error" {
		return nil
	}
	message := firstNonEmpty(
		stringFromAny(response.Args["description"]),
		stringFromAny(response.Args["message"]),
		stringFromAny(response.Args["type"]),
		"Stalwart JMAP 方法调用失败",
	)
	return errors.New(message)
}

func stalwartSetErrors(args map[string]any, key string, objectKey string) error {
	failures := mapFromAny(args[key])
	if len(failures) == 0 {
		return nil
	}
	item := mapFromAny(failures[objectKey])
	if len(item) == 0 {
		for _, value := range failures {
			item = mapFromAny(value)
			break
		}
	}
	message := firstNonEmpty(
		stringFromAny(item["description"]),
		stringFromAny(item["message"]),
		stringFromAny(item["type"]),
		fmt.Sprintf("Stalwart %s 失败", key),
	)
	return errors.New(message)
}

func syncMailboxLifecycle(ctx context.Context, config *AdminConfig, user UserRecord, action string, password string, idempotencyKey string) (MailboxLifecycleResult, error) {
	server := normalizeMailServerConfig(config.Mailbox.Server)
	if !server.Enabled {
		return MailboxLifecycleResult{}, errors.New("邮件服务未启用，无法同步真实账号生命周期")
	}
	endpoint := mailServerLifecycleEndpoint(server)
	localPart, domain := splitEmailAddress(user.Email)
	if localPart == "" || domain == "" {
		return MailboxLifecycleResult{}, errors.New("邮箱地址无效，无法同步邮件服务")
	}
	payload := MailboxLifecyclePayload{
		Action:      action,
		AccountID:   user.ID,
		Email:       user.Email,
		LocalPart:   localPart,
		Domain:      domain,
		ExternalID:  user.MailboxExternalID,
		DisplayName: user.DisplayName,
		Phone:       user.Phone,
		Password:    password,
		Metadata: map[string]any{
			"provider": "infinitemail",
		},
	}
	if endpoint == "" {
		if stalwartJMAPReady(server) {
			result, err := syncStalwartMailboxLifecycle(ctx, server, payload)
			if err != nil {
				return MailboxLifecycleResult{}, err
			}
			config.Mailbox.Server.LastLifecycleSyncAt = nowISO()
			return result, nil
		}
		return MailboxLifecycleResult{}, missingMailDataPlaneEndpointError("账号生命周期接口")
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return MailboxLifecycleResult{}, fmt.Errorf("账号同步请求编码失败: %w", err)
	}
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(raw))
	if err != nil {
		return MailboxLifecycleResult{}, errors.New("账号生命周期接口地址无效")
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Idempotency-Key", idempotencyKey)
	applyMailServerAuthHeaders(req, server)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return MailboxLifecycleResult{}, fmt.Errorf("账号生命周期接口调用失败: %w", err)
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(res.Body, 4096))
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		message := strings.TrimSpace(string(body))
		if message == "" {
			message = http.StatusText(res.StatusCode)
		}
		return MailboxLifecycleResult{}, fmt.Errorf("账号生命周期接口返回 HTTP %d: %s", res.StatusCode, message)
	}
	result := parseMailboxLifecycleResult(body)
	result.ExternalID = firstNonEmpty(result.ExternalID, user.MailboxExternalID, "remote_"+user.ID)
	result.Status = firstNonEmpty(result.Status, "synced")
	config.Mailbox.Server.LastLifecycleSyncAt = nowISO()
	return result, nil
}

func parseMailboxLifecycleResult(raw []byte) MailboxLifecycleResult {
	if len(raw) == 0 {
		return MailboxLifecycleResult{}
	}
	var result MailboxLifecycleResult
	_ = json.Unmarshal(raw, &result)
	var generic map[string]any
	if err := json.Unmarshal(raw, &generic); err == nil {
		result.ExternalID = firstNonEmpty(
			result.ExternalID,
			stringFromAny(generic["externalId"]),
			stringFromAny(generic["externalID"]),
			stringFromAny(generic["principalId"]),
			stringFromAny(generic["principalID"]),
			stringFromAny(generic["id"]),
			stringFromAny(generic["accountId"]),
		)
		result.Status = firstNonEmpty(result.Status, stringFromAny(generic["status"]))
		result.Message = firstNonEmpty(result.Message, stringFromAny(generic["message"]))
	}
	return result
}

func fetchMailboxMessageList(ctx context.Context, config AdminConfig, user UserRecord, folderID string, filter string, search string) (MailboxMessageListResult, error) {
	server := normalizeMailServerConfig(config.Mailbox.Server)
	endpoint := mailServerMessageListEndpoint(server)
	if endpoint == "" {
		if imapInboundReady(server) {
			return fetchIMAPMessageList(ctx, server, user, folderID, filter, search)
		}
		return MailboxMessageListResult{}, missingMailDataPlaneEndpointError("收件列表接口")
	}
	localPart, domain := splitEmailAddress(user.Email)
	if localPart == "" || domain == "" {
		return MailboxMessageListResult{}, errors.New("邮箱地址无效，无法同步邮件服务")
	}
	parsed, err := url.Parse(endpoint)
	if err != nil {
		return MailboxMessageListResult{}, errors.New("收件列表接口地址无效")
	}
	query := parsed.Query()
	query.Set("accountId", user.ID)
	query.Set("email", user.Email)
	query.Set("localPart", localPart)
	query.Set("domain", domain)
	query.Set("folderId", folderID)
	if filter != "" {
		query.Set("filter", filter)
	}
	if search != "" {
		query.Set("search", search)
	}
	parsed.RawQuery = query.Encode()

	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, parsed.String(), nil)
	if err != nil {
		return MailboxMessageListResult{}, errors.New("收件列表接口地址无效")
	}
	req.Header.Set("Accept", "application/json")
	applyMailServerAuthHeaders(req, server)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return MailboxMessageListResult{}, fmt.Errorf("收件列表接口调用失败: %w", err)
	}
	defer res.Body.Close()
	bodyRaw, _ := io.ReadAll(io.LimitReader(res.Body, 256*1024))
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		message := strings.TrimSpace(string(bodyRaw))
		if message == "" {
			message = http.StatusText(res.StatusCode)
		}
		return MailboxMessageListResult{}, fmt.Errorf("收件列表接口返回 HTTP %d: %s", res.StatusCode, message)
	}
	result := parseMailboxMessageListResult(bodyRaw, user, folderID)
	result.Called = true
	return result, nil
}

func fetchMailboxMessageDetail(ctx context.Context, config AdminConfig, user UserRecord, messageID string) (MailMessage, bool, error) {
	server := normalizeMailServerConfig(config.Mailbox.Server)
	endpoint := mailServerMessageDetailEndpoint(server, messageID)
	if endpoint == "" {
		if imapInboundReady(server) {
			return fetchIMAPMessageDetail(ctx, server, user, messageID)
		}
		return MailMessage{}, false, missingMailDataPlaneEndpointError("邮件详情接口")
	}
	localPart, domain := splitEmailAddress(user.Email)
	if localPart == "" || domain == "" {
		return MailMessage{}, false, errors.New("邮箱地址无效，无法同步邮件服务")
	}
	parsed, err := url.Parse(endpoint)
	if err != nil {
		return MailMessage{}, false, errors.New("邮件详情接口地址无效")
	}
	query := parsed.Query()
	query.Set("accountId", user.ID)
	query.Set("email", user.Email)
	query.Set("localPart", localPart)
	query.Set("domain", domain)
	parsed.RawQuery = query.Encode()

	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, parsed.String(), nil)
	if err != nil {
		return MailMessage{}, false, errors.New("邮件详情接口地址无效")
	}
	req.Header.Set("Accept", "application/json")
	applyMailServerAuthHeaders(req, server)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return MailMessage{}, true, fmt.Errorf("邮件详情接口调用失败: %w", err)
	}
	defer res.Body.Close()
	bodyRaw, _ := io.ReadAll(io.LimitReader(res.Body, 256*1024))
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		message := strings.TrimSpace(string(bodyRaw))
		if message == "" {
			message = http.StatusText(res.StatusCode)
		}
		return MailMessage{}, true, fmt.Errorf("邮件详情接口返回 HTTP %d: %s", res.StatusCode, message)
	}
	message := parseMailboxMessageDetailResult(bodyRaw)
	if strings.TrimSpace(message.ID) == "" {
		message.ID = strings.TrimSpace(messageID)
	}
	message = normalizeRemoteMailMessage(message, user, message.Folder, time.Now())
	return message, true, nil
}

func syncMailboxMessageStar(ctx context.Context, config AdminConfig, user UserRecord, messageID string, starred bool) (bool, error) {
	server := normalizeMailServerConfig(config.Mailbox.Server)
	endpoint := mailServerMessageStarEndpoint(server, messageID)
	if endpoint == "" {
		if imapInboundReady(server) {
			return syncIMAPMessageStar(ctx, server, user, messageID, starred)
		}
		return false, missingMailDataPlaneEndpointError("星标接口")
	}
	payload := MailboxMessageActionPayload{
		Action:    "star",
		MessageID: messageID,
		Starred:   &starred,
		Metadata: map[string]any{
			"provider": "infinitemail",
		},
	}
	return syncMailboxMessageAction(ctx, server, user, endpoint, http.MethodPatch, payload)
}

func syncMailboxMessageMove(ctx context.Context, config AdminConfig, user UserRecord, messageID string, previousFolder string, targetFolder string) (bool, error) {
	server := normalizeMailServerConfig(config.Mailbox.Server)
	endpoint := mailServerMessageMoveEndpoint(server, messageID)
	if endpoint == "" {
		if imapInboundReady(server) {
			return syncIMAPMessageMove(ctx, server, user, messageID, previousFolder, targetFolder)
		}
		return false, missingMailDataPlaneEndpointError("移动接口")
	}
	payload := MailboxMessageActionPayload{
		Action:         "move",
		MessageID:      messageID,
		PreviousFolder: previousFolder,
		TargetFolder:   targetFolder,
		Metadata: map[string]any{
			"provider": "infinitemail",
		},
	}
	return syncMailboxMessageAction(ctx, server, user, endpoint, http.MethodPost, payload)
}

func syncMailboxMessageRead(ctx context.Context, config AdminConfig, user UserRecord, messageID string, isUnread bool) (bool, error) {
	server := normalizeMailServerConfig(config.Mailbox.Server)
	endpoint := mailServerMessageReadEndpoint(server, messageID)
	if endpoint == "" {
		if imapInboundReady(server) {
			return syncIMAPMessageRead(ctx, server, user, messageID, isUnread)
		}
		return false, missingMailDataPlaneEndpointError("已读接口")
	}
	read := !isUnread
	payload := MailboxMessageActionPayload{
		Action:    "read",
		MessageID: messageID,
		IsUnread:  &isUnread,
		Read:      &read,
		Metadata: map[string]any{
			"provider": "infinitemail",
		},
	}
	return syncMailboxMessageAction(ctx, server, user, endpoint, http.MethodPatch, payload)
}

func syncMailboxMessageAction(ctx context.Context, server MailServerConfig, user UserRecord, endpoint string, method string, payload MailboxMessageActionPayload) (bool, error) {
	localPart, domain := splitEmailAddress(user.Email)
	if localPart == "" || domain == "" {
		return true, errors.New("邮箱地址无效，无法同步邮件服务")
	}
	payload.AccountID = user.ID
	payload.Email = user.Email
	payload.LocalPart = localPart
	payload.Domain = domain
	raw, err := json.Marshal(payload)
	if err != nil {
		return true, fmt.Errorf("邮件操作请求编码失败: %w", err)
	}
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, method, endpoint, bytes.NewReader(raw))
	if err != nil {
		return true, errors.New("邮件操作接口地址无效")
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Idempotency-Key", payload.Action+"-"+payload.MessageID)
	applyMailServerAuthHeaders(req, server)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return true, fmt.Errorf("邮件操作接口调用失败: %w", err)
	}
	defer res.Body.Close()
	bodyRaw, _ := io.ReadAll(io.LimitReader(res.Body, 4096))
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		message := strings.TrimSpace(string(bodyRaw))
		if message == "" {
			message = http.StatusText(res.StatusCode)
		}
		return true, fmt.Errorf("邮件操作接口返回 HTTP %d: %s", res.StatusCode, message)
	}
	return true, nil
}

func parseMailboxMessageListResult(raw []byte, user UserRecord, fallbackFolder string) MailboxMessageListResult {
	payload := unwrapEnvelopeData(raw)
	result := MailboxMessageListResult{
		Items:      []MailMessage{},
		NextCursor: nil,
	}
	rawItems := extractMessageItemsRaw(payload)
	now := time.Now()
	for _, rawItem := range rawItems {
		message := parseMailMessageRaw(rawItem)
		message = normalizeRemoteMailMessage(message, user, fallbackFolder, now)
		result.Items = append(result.Items, message)
	}
	var generic map[string]any
	if err := json.Unmarshal(payload, &generic); err == nil {
		result.HasMore = boolFromAny(generic["hasMore"])
		if result.HasMore == false {
			result.HasMore = boolFromAny(generic["has_more"])
		}
		if cursor := stringFromAny(generic["nextCursor"]); cursor != "" {
			result.NextCursor = cursor
		} else if cursor := stringFromAny(generic["next_cursor"]); cursor != "" {
			result.NextCursor = cursor
		}
	}
	return result
}

func parseMailboxMessageDetailResult(raw []byte) MailMessage {
	payload := unwrapEnvelopeData(raw)
	var generic map[string]json.RawMessage
	if err := json.Unmarshal(payload, &generic); err == nil {
		for _, key := range []string{"message", "item", "record"} {
			if rawMessage, ok := generic[key]; ok {
				return parseMailMessageRaw(rawMessage)
			}
		}
	}
	return parseMailMessageRaw(payload)
}

func extractMessageItemsRaw(payload []byte) []json.RawMessage {
	var direct []json.RawMessage
	if err := json.Unmarshal(payload, &direct); err == nil {
		return direct
	}
	var generic map[string]json.RawMessage
	if err := json.Unmarshal(payload, &generic); err != nil {
		return []json.RawMessage{}
	}
	for _, key := range []string{"items", "messages", "records", "list"} {
		raw, ok := generic[key]
		if !ok {
			continue
		}
		var items []json.RawMessage
		if err := json.Unmarshal(raw, &items); err == nil {
			return items
		}
	}
	return []json.RawMessage{}
}

func parseMailMessageRaw(raw []byte) MailMessage {
	var message MailMessage
	_ = json.Unmarshal(raw, &message)
	var generic map[string]any
	if err := json.Unmarshal(raw, &generic); err != nil {
		return message
	}
	message.ID = firstNonEmpty(message.ID, stringFromAny(generic["id"]), stringFromAny(generic["messageId"]), stringFromAny(generic["message_id"]))
	message.ThreadID = firstNonEmpty(message.ThreadID, stringFromAny(generic["threadId"]), stringFromAny(generic["thread_id"]))
	message.Folder = firstNonEmpty(message.Folder, stringFromAny(generic["folder"]), stringFromAny(generic["folderId"]), stringFromAny(generic["folder_id"]))
	message.PreviousFolder = firstNonEmpty(message.PreviousFolder, stringFromAny(generic["previousFolder"]), stringFromAny(generic["previous_folder"]))
	message.Sender = firstNonEmpty(message.Sender, stringFromAny(generic["sender"]), stringFromAny(generic["fromName"]), stringFromAny(generic["from_name"]))
	message.SenderEmail = firstNonEmpty(message.SenderEmail, stringFromAny(generic["senderEmail"]), stringFromAny(generic["sender_email"]), stringFromAny(generic["from"]), stringFromAny(generic["fromEmail"]), stringFromAny(generic["from_email"]))
	message.Recipients = firstNonEmptySlice(message.Recipients, stringSliceFromAny(generic["recipients"]), stringSliceFromAny(generic["to"]))
	message.Avatar = firstNonEmpty(message.Avatar, stringFromAny(generic["avatar"]))
	message.Role = firstNonEmpty(message.Role, stringFromAny(generic["role"]))
	message.Subject = firstNonEmpty(message.Subject, stringFromAny(generic["subject"]))
	message.Snippet = firstNonEmpty(message.Snippet, stringFromAny(generic["snippet"]), stringFromAny(generic["preview"]))
	message.Time = firstNonEmpty(message.Time, stringFromAny(generic["time"]), stringFromAny(generic["timeLabel"]), stringFromAny(generic["time_label"]))
	message.DateTimeLabel = firstNonEmpty(message.DateTimeLabel, stringFromAny(generic["dateTimeLabel"]), stringFromAny(generic["date_time_label"]))
	message.SortAt = firstNonEmpty(message.SortAt, stringFromAny(generic["sortAt"]), stringFromAny(generic["sort_at"]), stringFromAny(generic["createdAt"]), stringFromAny(generic["created_at"]))
	message.SentAt = firstNonEmpty(message.SentAt, stringFromAny(generic["sentAt"]), stringFromAny(generic["sent_at"]))
	message.ReceivedAt = firstNonEmpty(message.ReceivedAt, stringFromAny(generic["receivedAt"]), stringFromAny(generic["received_at"]))
	message.IsUnread = message.IsUnread || boolFromAny(generic["isUnread"]) || boolFromAny(generic["is_unread"]) || boolFromAny(generic["unread"])
	message.IsStarred = message.IsStarred || boolFromAny(generic["isStarred"]) || boolFromAny(generic["is_starred"]) || boolFromAny(generic["starred"])
	message.HasAttachment = message.HasAttachment || boolFromAny(generic["hasAttachment"]) || boolFromAny(generic["has_attachment"])
	message.Tags = firstNonEmptySlice(message.Tags, stringSliceFromAny(generic["tags"]))
	message.IsOutgoing = message.IsOutgoing || boolFromAny(generic["isOutgoing"]) || boolFromAny(generic["is_outgoing"])
	message.Content = firstNonEmpty(message.Content, stringFromAny(generic["content"]), stringFromAny(generic["html"]), stringFromAny(generic["bodyHtml"]), stringFromAny(generic["body_html"]))
	message.Attachments = firstNonEmptyAnySlice(message.Attachments, anySliceFromAny(generic["attachments"]))
	message.Source = firstNonEmpty(message.Source, stringFromAny(generic["source"]))
	message.DeliveryStatus = firstNonEmpty(message.DeliveryStatus, stringFromAny(generic["deliveryStatus"]), stringFromAny(generic["delivery_status"]), stringFromAny(generic["status"]))
	message.AcceptedAt = firstNonEmpty(message.AcceptedAt, stringFromAny(generic["acceptedAt"]), stringFromAny(generic["accepted_at"]))
	message.ProviderMessageID = firstNonEmpty(message.ProviderMessageID, stringFromAny(generic["providerMessageId"]), stringFromAny(generic["provider_message_id"]), stringFromAny(generic["messageProviderId"]))
	message.DeliveryError = firstNonEmpty(message.DeliveryError, stringFromAny(generic["deliveryError"]), stringFromAny(generic["delivery_error"]), stringFromAny(generic["error"]))
	return message
}

func relayMailboxMessage(ctx context.Context, config AdminConfig, user UserRecord, message MailMessage, sendPayload SendMessagePayload, draftPayload SaveDraftPayload, mode string) (MailboxMessageRelayResult, error) {
	server := normalizeMailServerConfig(config.Mailbox.Server)
	var endpoint string
	switch mode {
	case "send":
		endpoint = mailServerMessageSendEndpoint(server)
	case "draft":
		endpoint = mailServerDraftEndpoint(server)
	default:
		return MailboxMessageRelayResult{}, nil
	}
	if endpoint == "" {
		if mode == "send" && smtpOutboundReady(server) {
			return sendSMTPMailboxMessage(ctx, server, user, message, sendPayload)
		}
		if mode == "draft" && imapInboundReady(server) {
			return saveIMAPDraft(ctx, server, user, message, draftPayload)
		}
		if mode == "draft" {
			return MailboxMessageRelayResult{}, missingMailDataPlaneEndpointError("草稿接口")
		}
		return MailboxMessageRelayResult{}, missingMailDataPlaneEndpointError("发信接口")
	}
	localPart, domain := splitEmailAddress(user.Email)
	if localPart == "" || domain == "" {
		return MailboxMessageRelayResult{}, errors.New("邮箱地址无效，无法同步邮件服务")
	}
	body := sendPayload.Body
	recipients := sendPayload.Recipients
	cc := sendPayload.CC
	bcc := sendPayload.BCC
	subject := sendPayload.Subject
	attachments := sendPayload.Attachments
	source := firstNonEmpty(sendPayload.Source, "manual")
	replyToMessageID := sendPayload.ReplyToMessageID
	autosave := false
	draftID := ""
	if mode == "draft" {
		body = draftPayload.Body
		recipients = draftPayload.Recipients
		cc = draftPayload.CC
		bcc = draftPayload.BCC
		subject = draftPayload.Subject
		attachments = draftPayload.Attachments
		source = "draft"
		autosave = draftPayload.Autosave
		draftID = message.ID
	}
	relayPayload := MailboxMessageRelayPayload{
		AccountID:        user.ID,
		Email:            user.Email,
		LocalPart:        localPart,
		Domain:           domain,
		DisplayName:      user.DisplayName,
		Phone:            user.Phone,
		MessageID:        message.ID,
		DraftID:          draftID,
		Recipients:       recipients,
		CC:               cc,
		BCC:              bcc,
		Subject:          subject,
		Body:             body,
		Attachments:      attachments,
		Source:           source,
		ReplyToMessageID: replyToMessageID,
		Autosave:         autosave,
		Metadata: map[string]any{
			"provider": "infinitemail",
			"mode":     mode,
		},
	}
	raw, err := json.Marshal(relayPayload)
	if err != nil {
		return MailboxMessageRelayResult{}, fmt.Errorf("邮件请求编码失败: %w", err)
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(raw))
	if err != nil {
		return MailboxMessageRelayResult{}, errors.New("邮件数据接口地址无效")
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Idempotency-Key", message.ID)
	applyMailServerAuthHeaders(req, server)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return MailboxMessageRelayResult{}, fmt.Errorf("邮件数据接口调用失败: %w", err)
	}
	defer res.Body.Close()
	bodyRaw, _ := io.ReadAll(io.LimitReader(res.Body, 64*1024))
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		message := strings.TrimSpace(string(bodyRaw))
		if message == "" {
			message = http.StatusText(res.StatusCode)
		}
		return MailboxMessageRelayResult{}, fmt.Errorf("邮件数据接口返回 HTTP %d: %s", res.StatusCode, message)
	}
	return parseMailboxMessageRelayResult(bodyRaw), nil
}

func smtpOutboundReady(config MailServerConfig) bool {
	config.SMTPHost = strings.TrimSpace(config.SMTPHost)
	return config.SMTPEnabled && config.SMTPHost != "" && config.SMTPPort > 0
}

func imapInboundReady(config MailServerConfig) bool {
	return config.IMAPEnabled &&
		strings.TrimSpace(config.IMAPHost) != "" &&
		config.IMAPPort > 0
}

type imapFetchMessage struct {
	UID          uint32
	Flags        []string
	InternalDate string
	SizeBytes    int64
	Raw          []byte
}

type imapMIMEContent struct {
	Text          string
	HTML          string
	Attachments   []any
	HasAttachment bool
}

type imapClient struct {
	conn   net.Conn
	reader *bufio.Reader
	writer *bufio.Writer
	host   string
	tag    int
}

func fetchIMAPMessageList(ctx context.Context, config MailServerConfig, user UserRecord, folderID string, filter string, search string) (MailboxMessageListResult, error) {
	client, err := dialIMAP(ctx, config, user)
	if err != nil {
		return MailboxMessageListResult{}, err
	}
	defer client.logout()

	folders := []string{normalizeMessageFolder(folderID, "inbox")}
	criteria := "ALL"
	if filter == "unread" {
		criteria = "UNSEEN"
	} else if filter == "important" || folders[0] == "starred" {
		criteria = "FLAGGED"
	}
	if folders[0] == "starred" {
		folders = []string{"inbox", "sent", "archive"}
	}

	items := []MailMessage{}
	hasMore := false
	for _, folder := range folders {
		if _, err := client.selectMailbox(folder); err != nil {
			if len(folders) == 1 {
				return MailboxMessageListResult{Called: true, Items: []MailMessage{}}, nil
			}
			continue
		}
		uids, err := client.uidSearch(criteria)
		if err != nil {
			return MailboxMessageListResult{}, err
		}
		if len(uids) == 0 {
			continue
		}
		if len(uids) > 50 {
			hasMore = true
			uids = uids[len(uids)-50:]
		}
		fetched, err := client.uidFetchRaw(imapUIDSet(uids))
		if err != nil {
			return MailboxMessageListResult{}, err
		}
		for _, item := range fetched {
			message, err := parseIMAPMailMessage(item, folder, user)
			if err != nil {
				continue
			}
			items = append(items, message)
		}
	}
	items = filterMessages(items, normalizeMessageFolder(folderID, "inbox"), filter, search)
	return MailboxMessageListResult{Called: true, Items: items, HasMore: hasMore}, nil
}

func fetchIMAPMessageDetail(ctx context.Context, config MailServerConfig, user UserRecord, messageID string) (MailMessage, bool, error) {
	folder, uid, ok := decodeIMAPMessageID(messageID)
	if !ok {
		return MailMessage{}, true, errors.New("IMAP 邮件编号无效，请刷新邮件列表后重试")
	}
	client, err := dialIMAP(ctx, config, user)
	if err != nil {
		return MailMessage{}, true, err
	}
	defer client.logout()
	if _, err := client.selectMailbox(folder); err != nil {
		return MailMessage{}, true, err
	}
	items, err := client.uidFetchRaw(strconv.FormatUint(uint64(uid), 10))
	if err != nil {
		return MailMessage{}, true, err
	}
	if len(items) == 0 {
		return MailMessage{}, true, errors.New("IMAP 邮件不存在")
	}
	message, err := parseIMAPMailMessage(items[0], folder, user)
	if err != nil {
		return MailMessage{}, true, err
	}
	return message, true, nil
}

func syncIMAPMessageStar(ctx context.Context, config MailServerConfig, user UserRecord, messageID string, starred bool) (bool, error) {
	op := "+FLAGS.SILENT"
	if !starred {
		op = "-FLAGS.SILENT"
	}
	return syncIMAPMessageFlags(ctx, config, user, messageID, op, []string{`\Flagged`})
}

func syncIMAPMessageRead(ctx context.Context, config MailServerConfig, user UserRecord, messageID string, isUnread bool) (bool, error) {
	op := "+FLAGS.SILENT"
	if isUnread {
		op = "-FLAGS.SILENT"
	}
	return syncIMAPMessageFlags(ctx, config, user, messageID, op, []string{`\Seen`})
}

func syncIMAPMessageFlags(ctx context.Context, config MailServerConfig, user UserRecord, messageID string, operation string, flags []string) (bool, error) {
	folder, uid, ok := decodeIMAPMessageID(messageID)
	if !ok {
		return true, errors.New("IMAP 邮件编号无效，请刷新邮件列表后重试")
	}
	client, err := dialIMAP(ctx, config, user)
	if err != nil {
		return true, err
	}
	defer client.logout()
	if _, err := client.selectMailbox(folder); err != nil {
		return true, err
	}
	command := fmt.Sprintf("UID STORE %d %s (%s)", uid, operation, strings.Join(flags, " "))
	if _, err := client.command(command); err != nil {
		return true, fmt.Errorf("IMAP 更新邮件标记失败: %w", err)
	}
	return true, nil
}

func syncIMAPMessageMove(ctx context.Context, config MailServerConfig, user UserRecord, messageID string, previousFolder string, targetFolder string) (bool, error) {
	folder, uid, ok := decodeIMAPMessageID(messageID)
	if !ok {
		return true, errors.New("IMAP 邮件编号无效，请刷新邮件列表后重试")
	}
	if previousFolder != "" {
		folder = normalizeMessageFolder(previousFolder, folder)
	}
	client, err := dialIMAP(ctx, config, user)
	if err != nil {
		return true, err
	}
	defer client.logout()
	if _, err := client.selectMailbox(folder); err != nil {
		return true, err
	}
	moveErrors := []string{}
	for _, mailbox := range imapFolderCandidates(targetFolder) {
		if _, err := client.command(fmt.Sprintf("UID MOVE %d %s", uid, imapQuote(mailbox))); err == nil {
			return true, nil
		} else {
			moveErrors = append(moveErrors, err.Error())
		}
		if _, err := client.command(fmt.Sprintf("UID COPY %d %s", uid, imapQuote(mailbox))); err == nil {
			_, _ = client.command(fmt.Sprintf("UID STORE %d +FLAGS.SILENT (\\Deleted)", uid))
			_, _ = client.command("EXPUNGE")
			return true, nil
		}
	}
	return true, fmt.Errorf("IMAP 移动邮件失败: %s", strings.Join(moveErrors, "；"))
}

func saveIMAPDraft(ctx context.Context, config MailServerConfig, user UserRecord, message MailMessage, payload SaveDraftPayload) (MailboxMessageRelayResult, error) {
	raw, err := buildSMTPMessage(user, message, SendMessagePayload{
		Recipients:  payload.Recipients,
		CC:          payload.CC,
		BCC:         payload.BCC,
		Subject:     firstNonEmpty(payload.Subject, message.Subject),
		Body:        payload.Body,
		Attachments: payload.Attachments,
		Source:      "draft",
	})
	if err != nil {
		return MailboxMessageRelayResult{}, err
	}
	if err := appendIMAPMessage(ctx, config, user, "drafts", raw, []string{`\Draft`}); err != nil {
		return MailboxMessageRelayResult{}, err
	}
	message.ProviderMessageID = firstNonEmpty(message.ProviderMessageID, "imap-draft-"+message.ID)
	message.DeliveryStatus = "draft"
	return MailboxMessageRelayResult{Draft: message, Status: "draft", ProviderMessageID: message.ProviderMessageID}, nil
}

func appendIMAPMessage(ctx context.Context, config MailServerConfig, user UserRecord, folder string, raw []byte, flags []string) error {
	client, err := dialIMAP(ctx, config, user)
	if err != nil {
		return err
	}
	defer client.logout()
	errorsList := []string{}
	for _, mailbox := range imapFolderCandidates(folder) {
		if err := client.appendMessage(mailbox, raw, flags); err == nil {
			return nil
		} else {
			errorsList = append(errorsList, mailbox+": "+err.Error())
		}
	}
	return fmt.Errorf("IMAP 归档邮件失败: %s", strings.Join(errorsList, "；"))
}

func testIMAPConnection(ctx context.Context, config MailServerConfig, user UserRecord) error {
	client, err := dialIMAP(ctx, config, user)
	if err != nil {
		return err
	}
	defer client.logout()
	if _, err := client.selectMailbox("inbox"); err != nil {
		return fmt.Errorf("IMAP INBOX 不可用: %w", err)
	}
	return nil
}

func imapConnectionTestNeedsOnlyProtocol(config MailServerConfig) bool {
	config = normalizeMailServerConfig(config)
	return strings.TrimSpace(config.IMAPPassword) == "" || strings.Contains(config.IMAPUsername, "{")
}

func testIMAPProtocolConnection(ctx context.Context, config MailServerConfig) error {
	config = normalizeMailServerConfig(config)
	if !imapInboundReady(config) {
		return errors.New("IMAP 收件未配置")
	}
	ctx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()
	dialer := net.Dialer{}
	address := net.JoinHostPort(config.IMAPHost, strconv.Itoa(config.IMAPPort))
	var conn net.Conn
	var err error
	if config.IMAPTLSMode == "tls" || (config.IMAPTLSMode == "auto" && config.IMAPPort == 993) {
		tlsDialer := tls.Dialer{NetDialer: &dialer, Config: &tls.Config{ServerName: config.IMAPHost, MinVersion: tls.VersionTLS12}}
		conn, err = tlsDialer.DialContext(ctx, "tcp", address)
	} else {
		conn, err = dialer.DialContext(ctx, "tcp", address)
	}
	if err != nil {
		return fmt.Errorf("IMAP 连接失败: %w", err)
	}
	_ = conn.SetDeadline(time.Now().Add(4 * time.Second))
	client := &imapClient{conn: conn, reader: bufio.NewReader(conn), writer: bufio.NewWriter(conn), host: config.IMAPHost}
	defer client.logout()
	greeting, err := client.reader.ReadString('\n')
	if err != nil {
		return fmt.Errorf("IMAP 握手失败: %w", err)
	}
	if !strings.HasPrefix(strings.ToUpper(strings.TrimSpace(greeting)), "* OK") {
		return fmt.Errorf("IMAP 服务拒绝连接: %s", strings.TrimSpace(greeting))
	}
	if config.IMAPTLSMode == "starttls" || config.IMAPTLSMode == "auto" {
		lines, _ := client.command("CAPABILITY")
		hasStartTLS := imapLinesContainCapability(lines, "STARTTLS")
		if hasStartTLS {
			if _, err := client.command("STARTTLS"); err != nil {
				return fmt.Errorf("IMAP STARTTLS 失败: %w", err)
			}
			tlsConn := tls.Client(conn, &tls.Config{ServerName: config.IMAPHost, MinVersion: tls.VersionTLS12})
			if err := tlsConn.HandshakeContext(ctx); err != nil {
				return fmt.Errorf("IMAP TLS 握手失败: %w", err)
			}
			client.conn = tlsConn
			client.reader = bufio.NewReader(tlsConn)
			client.writer = bufio.NewWriter(tlsConn)
		} else if config.IMAPTLSMode == "starttls" {
			return errors.New("IMAP 服务未提供 STARTTLS")
		}
	}
	return nil
}

func dialIMAP(ctx context.Context, config MailServerConfig, user UserRecord) (*imapClient, error) {
	config = normalizeMailServerConfig(config)
	if !imapInboundReady(config) {
		return nil, errors.New("IMAP 收件未配置")
	}
	username, password, err := resolveIMAPCredentials(config, user)
	if err != nil {
		return nil, err
	}
	address := net.JoinHostPort(config.IMAPHost, strconv.Itoa(config.IMAPPort))
	dialer := net.Dialer{}
	var conn net.Conn
	if config.IMAPTLSMode == "tls" || (config.IMAPTLSMode == "auto" && config.IMAPPort == 993) {
		tlsDialer := tls.Dialer{NetDialer: &dialer, Config: &tls.Config{ServerName: config.IMAPHost, MinVersion: tls.VersionTLS12}}
		conn, err = tlsDialer.DialContext(ctx, "tcp", address)
	} else {
		conn, err = dialer.DialContext(ctx, "tcp", address)
	}
	if err != nil {
		return nil, fmt.Errorf("IMAP 连接失败: %w", err)
	}
	_ = conn.SetDeadline(time.Now().Add(20 * time.Second))
	client := &imapClient{conn: conn, reader: bufio.NewReader(conn), writer: bufio.NewWriter(conn), host: config.IMAPHost}
	greeting, err := client.reader.ReadString('\n')
	if err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("IMAP 握手失败: %w", err)
	}
	if !strings.HasPrefix(strings.ToUpper(strings.TrimSpace(greeting)), "* OK") {
		_ = conn.Close()
		return nil, fmt.Errorf("IMAP 服务拒绝连接: %s", strings.TrimSpace(greeting))
	}
	if config.IMAPTLSMode == "starttls" || config.IMAPTLSMode == "auto" {
		lines, _ := client.command("CAPABILITY")
		hasStartTLS := imapLinesContainCapability(lines, "STARTTLS")
		if hasStartTLS {
			if _, err := client.command("STARTTLS"); err != nil {
				_ = conn.Close()
				return nil, fmt.Errorf("IMAP STARTTLS 失败: %w", err)
			}
			tlsConn := tls.Client(conn, &tls.Config{ServerName: config.IMAPHost, MinVersion: tls.VersionTLS12})
			if err := tlsConn.HandshakeContext(ctx); err != nil {
				_ = conn.Close()
				return nil, fmt.Errorf("IMAP TLS 握手失败: %w", err)
			}
			client.conn = tlsConn
			client.reader = bufio.NewReader(tlsConn)
			client.writer = bufio.NewWriter(tlsConn)
		} else if config.IMAPTLSMode == "starttls" {
			_ = conn.Close()
			return nil, errors.New("IMAP 服务未提供 STARTTLS")
		}
	}
	if _, err := client.command("LOGIN " + imapQuote(username) + " " + imapQuote(password)); err != nil {
		_ = client.conn.Close()
		return nil, fmt.Errorf("IMAP 认证失败: %w", err)
	}
	return client, nil
}

func resolveIMAPCredentials(config MailServerConfig, user UserRecord) (string, string, error) {
	username := strings.TrimSpace(config.IMAPUsername)
	if username == "" {
		username = firstNonEmpty(user.MailboxUsername, user.Email)
	}
	username = renderMailboxCredentialTemplate(username, user)
	password := strings.TrimSpace(config.IMAPPassword)
	if password == "" {
		var err error
		password, err = openMailboxPasswordSecret(user.MailboxPasswordSecret)
		if err != nil {
			return "", "", err
		}
	}
	if strings.TrimSpace(username) == "" {
		return "", "", errors.New("IMAP 账号未配置，可使用 {email}、{localPart}、{domain}、{mailboxUsername} 模板")
	}
	if password == "" {
		return "", "", errors.New("IMAP 密码未配置，可配置 IMAP 主密码，或先完成邮箱开通以保存用户邮箱凭据")
	}
	return username, password, nil
}

func renderMailboxCredentialTemplate(template string, user UserRecord) string {
	localPart, domain := splitEmailAddress(user.Email)
	replacements := map[string]string{
		"{email}":           user.Email,
		"{localPart}":       localPart,
		"{domain}":          domain,
		"{accountId}":       user.ID,
		"{mailboxUsername}": firstNonEmpty(user.MailboxUsername, user.Email),
	}
	rendered := template
	for token, value := range replacements {
		rendered = strings.ReplaceAll(rendered, token, value)
	}
	return strings.TrimSpace(rendered)
}

func (client *imapClient) nextTag() string {
	client.tag += 1
	return fmt.Sprintf("A%04d", client.tag)
}

func (client *imapClient) command(command string) ([]string, error) {
	tag := client.nextTag()
	if _, err := client.writer.WriteString(tag + " " + command + "\r\n"); err != nil {
		return nil, err
	}
	if err := client.writer.Flush(); err != nil {
		return nil, err
	}
	return client.readResponse(tag)
}

func (client *imapClient) readResponse(tag string) ([]string, error) {
	lines := []string{}
	for {
		line, err := client.reader.ReadString('\n')
		if err != nil {
			return lines, err
		}
		line = strings.TrimRight(line, "\r\n")
		if strings.HasPrefix(line, tag+" ") {
			return lines, imapStatusError(line)
		}
		if literalSize, ok := imapLiteralSize(line); ok {
			if _, err := io.CopyN(io.Discard, client.reader, int64(literalSize)); err != nil {
				return lines, err
			}
		}
		lines = append(lines, line)
	}
}

func (client *imapClient) uidSearch(criteria string) ([]uint32, error) {
	criteria = strings.TrimSpace(criteria)
	if criteria == "" {
		criteria = "ALL"
	}
	lines, err := client.command("UID SEARCH " + criteria)
	if err != nil {
		return nil, fmt.Errorf("IMAP 搜索邮件失败: %w", err)
	}
	uids := []uint32{}
	for _, line := range lines {
		if !strings.HasPrefix(strings.ToUpper(line), "* SEARCH") {
			continue
		}
		for _, token := range strings.Fields(strings.TrimSpace(strings.TrimPrefix(line, "* SEARCH"))) {
			uid, err := strconv.ParseUint(token, 10, 32)
			if err == nil && uid > 0 {
				uids = append(uids, uint32(uid))
			}
		}
	}
	sort.Slice(uids, func(i, j int) bool { return uids[i] < uids[j] })
	return uids, nil
}

func (client *imapClient) uidFetchRaw(uidSet string) ([]imapFetchMessage, error) {
	uidSet = strings.TrimSpace(uidSet)
	if uidSet == "" {
		return nil, nil
	}
	tag := client.nextTag()
	command := fmt.Sprintf("%s UID FETCH %s (UID FLAGS INTERNALDATE RFC822.SIZE BODY.PEEK[])\r\n", tag, uidSet)
	if _, err := client.writer.WriteString(command); err != nil {
		return nil, err
	}
	if err := client.writer.Flush(); err != nil {
		return nil, err
	}
	items := []imapFetchMessage{}
	for {
		line, err := client.reader.ReadString('\n')
		if err != nil {
			return items, err
		}
		line = strings.TrimRight(line, "\r\n")
		if strings.HasPrefix(line, tag+" ") {
			return items, imapStatusError(line)
		}
		if !strings.Contains(strings.ToUpper(line), " FETCH ") {
			continue
		}
		item := parseIMAPFetchMeta(line)
		literalSize, ok := imapLiteralSize(line)
		if !ok {
			continue
		}
		item.Raw = make([]byte, literalSize)
		if _, err := io.ReadFull(client.reader, item.Raw); err != nil {
			return items, err
		}
		for {
			rest, err := client.reader.ReadString('\n')
			if err != nil {
				return items, err
			}
			rest = strings.TrimRight(rest, "\r\n")
			if strings.TrimSpace(rest) == "" {
				continue
			}
			item = mergeIMAPFetchMeta(item, parseIMAPFetchMeta(rest))
			if strings.Contains(rest, ")") {
				break
			}
		}
		if item.UID > 0 && len(item.Raw) > 0 {
			items = append(items, item)
		}
	}
}

func (client *imapClient) selectMailbox(folder string) (string, error) {
	errorsList := []string{}
	for _, mailbox := range imapFolderCandidates(folder) {
		if _, err := client.command("SELECT " + imapQuote(mailbox)); err == nil {
			return mailbox, nil
		} else {
			errorsList = append(errorsList, mailbox+": "+err.Error())
		}
	}
	return "", fmt.Errorf("IMAP 文件夹不可用: %s", strings.Join(errorsList, "；"))
}

func (client *imapClient) appendMessage(mailbox string, raw []byte, flags []string) error {
	tag := client.nextTag()
	flagText := ""
	if len(flags) > 0 {
		flagText = " (" + strings.Join(flags, " ") + ")"
	}
	command := fmt.Sprintf("%s APPEND %s%s {%d}\r\n", tag, imapQuote(mailbox), flagText, len(raw))
	if _, err := client.writer.WriteString(command); err != nil {
		return err
	}
	if err := client.writer.Flush(); err != nil {
		return err
	}
	line, err := client.reader.ReadString('\n')
	if err != nil {
		return err
	}
	line = strings.TrimRight(line, "\r\n")
	if !strings.HasPrefix(line, "+") {
		if strings.HasPrefix(line, tag+" ") {
			return imapStatusError(line)
		}
		return fmt.Errorf("IMAP APPEND 未收到继续写入许可: %s", line)
	}
	if _, err := client.writer.Write(raw); err != nil {
		return err
	}
	if _, err := client.writer.WriteString("\r\n"); err != nil {
		return err
	}
	if err := client.writer.Flush(); err != nil {
		return err
	}
	_, err = client.readResponse(tag)
	if err != nil {
		return fmt.Errorf("IMAP APPEND 失败: %w", err)
	}
	return nil
}

func (client *imapClient) logout() {
	_, _ = client.command("LOGOUT")
	_ = client.conn.Close()
}

func parseIMAPFetchMeta(line string) imapFetchMessage {
	item := imapFetchMessage{}
	if match := regexp.MustCompile(`(?i)\bUID\s+(\d+)`).FindStringSubmatch(line); len(match) == 2 {
		if uid, err := strconv.ParseUint(match[1], 10, 32); err == nil {
			item.UID = uint32(uid)
		}
	}
	if match := regexp.MustCompile(`(?i)\bFLAGS\s+\(([^)]*)\)`).FindStringSubmatch(line); len(match) == 2 {
		item.Flags = strings.Fields(match[1])
	}
	if match := regexp.MustCompile(`(?i)\bINTERNALDATE\s+"([^"]+)"`).FindStringSubmatch(line); len(match) == 2 {
		item.InternalDate = match[1]
	}
	if match := regexp.MustCompile(`(?i)\bRFC822\.SIZE\s+(\d+)`).FindStringSubmatch(line); len(match) == 2 {
		if size, err := strconv.ParseInt(match[1], 10, 64); err == nil {
			item.SizeBytes = size
		}
	}
	return item
}

func mergeIMAPFetchMeta(current imapFetchMessage, next imapFetchMessage) imapFetchMessage {
	if current.UID == 0 {
		current.UID = next.UID
	}
	if len(current.Flags) == 0 {
		current.Flags = next.Flags
	}
	if current.InternalDate == "" {
		current.InternalDate = next.InternalDate
	}
	if current.SizeBytes == 0 {
		current.SizeBytes = next.SizeBytes
	}
	return current
}

func parseIMAPMailMessage(item imapFetchMessage, folder string, user UserRecord) (MailMessage, error) {
	parsed, err := netmail.ReadMessage(bytes.NewReader(item.Raw))
	if err != nil {
		return MailMessage{}, fmt.Errorf("IMAP 邮件解析失败: %w", err)
	}
	header := textproto.MIMEHeader(parsed.Header)
	subject := decodeMIMEHeader(header.Get("Subject"))
	fromName, fromEmail := parseMailboxAddress(decodeMIMEHeader(header.Get("From")))
	recipients := parseAddressHeaderList(header.Get("To"))
	cc := parseAddressHeaderList(header.Get("Cc"))
	recipients = append(recipients, cc...)
	date := parseMailHeaderDate(header.Get("Date"), item.InternalDate)
	content, err := parseMIMEContent(header, parsed.Body)
	if err != nil {
		content = imapMIMEContent{Text: "邮件正文解析失败: " + err.Error()}
	}
	folder = normalizeMessageFolder(folder, "inbox")
	bodyHTML := content.HTML
	if strings.TrimSpace(bodyHTML) == "" {
		bodyHTML = composeBodyHTML(MessageBodyPayload{Text: content.Text}, subject)
	}
	snippet := truncateRunes(firstNonEmpty(content.Text, stripHTML(bodyHTML), subject), 120)
	senderEmail := strings.ToLower(firstNonEmpty(fromEmail, "unknown@"+fallbackDomain(user.Email)))
	sender := firstNonEmpty(fromName, emailName(senderEmail), senderEmail)
	sortAt := date.Format(time.RFC3339)
	message := MailMessage{
		ID:                encodeIMAPMessageID(folder, item.UID),
		ThreadID:          strings.TrimSpace(header.Get("Message-ID")),
		Folder:            folder,
		PreviousFolder:    folder,
		Sender:            sender,
		SenderEmail:       senderEmail,
		Recipients:        firstNonEmptySlice(recipients, []string{user.Email}),
		Avatar:            avatarInitial(sender),
		Role:              "邮件联系人",
		Subject:           truncateRunes(firstNonEmpty(subject, "(无主题)"), 240),
		Snippet:           snippet,
		Time:              formatMailTimeLabel(sortAt),
		DateTimeLabel:     formatMailDateTimeLabel(sortAt),
		SortAt:            sortAt,
		SentAt:            sortAt,
		ReceivedAt:        sortAt,
		IsUnread:          !imapHasFlag(item.Flags, `\Seen`),
		IsStarred:         imapHasFlag(item.Flags, `\Flagged`),
		HasAttachment:     content.HasAttachment || len(content.Attachments) > 0,
		Tags:              []string{mailFolderTag(folder)},
		IsOutgoing:        folder == "sent" || folder == "drafts",
		Content:           bodyHTML,
		Attachments:       content.Attachments,
		Source:            "imap",
		DeliveryStatus:    "received",
		ProviderMessageID: strings.TrimSpace(header.Get("Message-ID")),
	}
	if message.IsOutgoing {
		message.DeliveryStatus = "accepted"
	}
	if folder == "drafts" {
		message.DeliveryStatus = "draft"
	}
	return normalizeRemoteMailMessage(message, user, folder, time.Now()), nil
}

func parseMIMEContent(header textproto.MIMEHeader, body io.Reader) (imapMIMEContent, error) {
	mediaType, params, _ := mime.ParseMediaType(header.Get("Content-Type"))
	mediaType = strings.ToLower(strings.TrimSpace(mediaType))
	if strings.HasPrefix(mediaType, "multipart/") {
		boundary := params["boundary"]
		if boundary == "" {
			return imapMIMEContent{}, errors.New("multipart 邮件缺少 boundary")
		}
		reader := multipart.NewReader(body, boundary)
		out := imapMIMEContent{Attachments: []any{}}
		for {
			part, err := reader.NextPart()
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				return out, err
			}
			partContent, err := parseMIMEContent(part.Header, part)
			if err != nil {
				continue
			}
			out.Text = firstNonEmpty(out.Text, partContent.Text)
			out.HTML = firstNonEmpty(out.HTML, partContent.HTML)
			out.Attachments = append(out.Attachments, partContent.Attachments...)
			out.HasAttachment = out.HasAttachment || partContent.HasAttachment
		}
		return out, nil
	}
	decoded := transferDecodedReader(body, header.Get("Content-Transfer-Encoding"))
	raw, err := io.ReadAll(io.LimitReader(decoded, maxMessageAttachmentBytes))
	if err != nil {
		return imapMIMEContent{}, err
	}
	disposition, dispositionParams, _ := mime.ParseMediaType(header.Get("Content-Disposition"))
	name := firstNonEmpty(dispositionParams["filename"], params["name"])
	if decodedName := decodeMIMEHeader(name); decodedName != "" {
		name = decodedName
	}
	isAttachment := strings.EqualFold(disposition, "attachment") || strings.TrimSpace(name) != ""
	if isAttachment {
		return imapMIMEContent{
			Attachments: []any{response{
				"id":          nextID("imap_att"),
				"name":        firstNonEmpty(name, "attachment"),
				"type":        firstNonEmpty(mediaType, "application/octet-stream"),
				"contentType": firstNonEmpty(mediaType, "application/octet-stream"),
				"sizeBytes":   len(raw),
				"source":      "imap",
			}},
			HasAttachment: true,
		}, nil
	}
	switch mediaType {
	case "text/html":
		return imapMIMEContent{HTML: string(raw)}, nil
	case "text/plain", "":
		return imapMIMEContent{Text: string(raw)}, nil
	default:
		return imapMIMEContent{HasAttachment: len(raw) > 0}, nil
	}
}

func transferDecodedReader(reader io.Reader, encoding string) io.Reader {
	switch strings.ToLower(strings.TrimSpace(encoding)) {
	case "base64":
		return base64.NewDecoder(base64.StdEncoding, reader)
	case "quoted-printable":
		return quotedprintable.NewReader(reader)
	default:
		return reader
	}
}

func parseMailHeaderDate(dateHeader string, internalDate string) time.Time {
	if parsed, err := netmail.ParseDate(strings.TrimSpace(dateHeader)); err == nil {
		return parsed
	}
	for _, layout := range []string{"02-Jan-2006 15:04:05 -0700", "_2-Jan-2006 15:04:05 -0700"} {
		if parsed, err := time.Parse(layout, strings.TrimSpace(internalDate)); err == nil {
			return parsed
		}
	}
	return time.Now()
}

func parseAddressHeaderList(value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	addresses, err := netmail.ParseAddressList(value)
	if err != nil {
		return []string{decodeMIMEHeader(value)}
	}
	out := []string{}
	for _, address := range addresses {
		if strings.TrimSpace(address.Address) != "" {
			out = append(out, strings.ToLower(strings.TrimSpace(address.Address)))
		}
	}
	return out
}

func decodeMIMEHeader(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	decoded, err := (&mime.WordDecoder{}).DecodeHeader(value)
	if err != nil {
		return value
	}
	return strings.TrimSpace(decoded)
}

func mailFolderTag(folder string) string {
	switch normalizeMessageFolder(folder, "inbox") {
	case "sent":
		return "已发送"
	case "drafts":
		return "草稿"
	case "trash":
		return "已删除"
	case "archive":
		return "归档"
	default:
		return "收件"
	}
}

func imapHasFlag(flags []string, expected string) bool {
	for _, flag := range flags {
		if strings.EqualFold(flag, expected) {
			return true
		}
	}
	return false
}

func encodeIMAPMessageID(folder string, uid uint32) string {
	value := normalizeMessageFolder(folder, "inbox") + "|" + strconv.FormatUint(uint64(uid), 10)
	return "imap-" + base64.RawURLEncoding.EncodeToString([]byte(value))
}

func decodeIMAPMessageID(id string) (string, uint32, bool) {
	id = strings.TrimSpace(id)
	if !strings.HasPrefix(id, "imap-") {
		return "", 0, false
	}
	raw, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(id, "imap-"))
	if err != nil {
		return "", 0, false
	}
	parts := strings.SplitN(string(raw), "|", 2)
	if len(parts) != 2 {
		return "", 0, false
	}
	uid, err := strconv.ParseUint(parts[1], 10, 32)
	if err != nil || uid == 0 {
		return "", 0, false
	}
	return normalizeMessageFolder(parts[0], "inbox"), uint32(uid), true
}

func imapUIDSet(uids []uint32) string {
	parts := make([]string, 0, len(uids))
	for _, uid := range uids {
		if uid > 0 {
			parts = append(parts, strconv.FormatUint(uint64(uid), 10))
		}
	}
	return strings.Join(parts, ",")
}

func imapFolderCandidates(folder string) []string {
	switch normalizeMessageFolder(folder, "inbox") {
	case "sent":
		return []string{"Sent", "Sent Messages", "Sent Items", "INBOX.Sent", "[Gmail]/Sent Mail"}
	case "drafts":
		return []string{"Drafts", "INBOX.Drafts", "[Gmail]/Drafts"}
	case "trash":
		return []string{"Trash", "Deleted Messages", "INBOX.Trash", "[Gmail]/Trash"}
	case "archive":
		return []string{"Archive", "INBOX.Archive", "[Gmail]/All Mail"}
	default:
		return []string{"INBOX"}
	}
}

func imapQuote(value string) string {
	replacer := strings.NewReplacer(`\`, `\\`, `"`, `\"`)
	return `"` + replacer.Replace(value) + `"`
}

func imapLiteralSize(line string) (int, bool) {
	line = strings.TrimSpace(line)
	start := strings.LastIndex(line, "{")
	end := strings.LastIndex(line, "}")
	if start < 0 || end != len(line)-1 || start >= end {
		return 0, false
	}
	raw := strings.TrimSuffix(line[start+1:end], "+")
	size, err := strconv.Atoi(raw)
	return size, err == nil && size >= 0
}

func imapStatusError(line string) error {
	fields := strings.Fields(line)
	if len(fields) < 2 {
		return errors.New(strings.TrimSpace(line))
	}
	status := strings.ToUpper(fields[1])
	if status == "OK" {
		return nil
	}
	message := strings.TrimSpace(strings.Join(fields[2:], " "))
	if message == "" {
		message = status
	}
	return errors.New(message)
}

func imapLinesContainCapability(lines []string, capability string) bool {
	capability = strings.ToUpper(strings.TrimSpace(capability))
	for _, line := range lines {
		for _, field := range strings.Fields(strings.ToUpper(line)) {
			if field == capability {
				return true
			}
		}
	}
	return false
}

func sendSMTPMailboxMessage(ctx context.Context, config MailServerConfig, user UserRecord, message MailMessage, payload SendMessagePayload) (MailboxMessageRelayResult, error) {
	if !smtpOutboundReady(config) {
		return MailboxMessageRelayResult{}, errors.New("SMTP 发信未配置")
	}
	recipients := smtpEnvelopeRecipients(payload)
	if len(recipients) == 0 {
		return MailboxMessageRelayResult{}, errors.New("请填写收件人")
	}
	raw, err := buildSMTPMessage(user, message, payload)
	if err != nil {
		return MailboxMessageRelayResult{}, err
	}
	if err := deliverSMTPMessage(ctx, config, user, user.Email, recipients, raw); err != nil {
		return MailboxMessageRelayResult{}, err
	}
	acceptedAt := nowISO()
	messageText := "SMTP accepted"
	if imapInboundReady(config) {
		if err := appendIMAPMessage(ctx, config, user, "sent", raw, []string{`\Seen`}); err != nil {
			messageText = "SMTP accepted，IMAP 已发送归档失败: " + err.Error()
		}
	}
	return MailboxMessageRelayResult{
		AcceptedAt:        acceptedAt,
		ProviderMessageID: "smtp-" + message.ID,
		Status:            "accepted",
		MessageText:       messageText,
	}, nil
}

func deliverSMTPMessage(ctx context.Context, config MailServerConfig, user UserRecord, from string, recipients []string, raw []byte) error {
	config = normalizeMailServerConfig(config)
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	dialer := net.Dialer{}
	address := net.JoinHostPort(config.SMTPHost, strconv.Itoa(config.SMTPPort))
	var conn net.Conn
	var err error
	if config.SMTPTLSMode == "tls" {
		tlsDialer := tls.Dialer{NetDialer: &dialer, Config: &tls.Config{ServerName: config.SMTPHost, MinVersion: tls.VersionTLS12}}
		conn, err = tlsDialer.DialContext(ctx, "tcp", address)
	} else {
		conn, err = dialer.DialContext(ctx, "tcp", address)
	}
	if err != nil {
		return fmt.Errorf("SMTP 连接失败: %w", err)
	}
	client, err := smtp.NewClient(conn, config.SMTPHost)
	if err != nil {
		_ = conn.Close()
		return fmt.Errorf("SMTP 握手失败: %w", err)
	}
	defer client.Close()
	if err := client.Hello("infinitemail"); err != nil {
		return fmt.Errorf("SMTP HELO 失败: %w", err)
	}
	if config.SMTPTLSMode != "none" && config.SMTPTLSMode != "tls" {
		if ok, _ := client.Extension("STARTTLS"); ok {
			tlsConfig := &tls.Config{ServerName: config.SMTPHost, MinVersion: tls.VersionTLS12}
			if err := client.StartTLS(tlsConfig); err != nil {
				return fmt.Errorf("SMTP STARTTLS 失败: %w", err)
			}
		} else if config.SMTPTLSMode == "starttls" {
			return errors.New("SMTP 服务未提供 STARTTLS")
		}
	}
	username, password, shouldAuth, err := resolveSMTPCredentials(config, user)
	if err != nil {
		return err
	}
	if shouldAuth {
		auth := smtp.PlainAuth("", username, password, config.SMTPHost)
		if err := client.Auth(auth); err != nil {
			return fmt.Errorf("SMTP 认证失败: %w", err)
		}
	}
	if err := client.Mail(from); err != nil {
		return fmt.Errorf("SMTP MAIL FROM 失败: %w", err)
	}
	for _, recipient := range recipients {
		if err := client.Rcpt(recipient); err != nil {
			return fmt.Errorf("SMTP RCPT TO %s 失败: %w", recipient, err)
		}
	}
	writer, err := client.Data()
	if err != nil {
		return fmt.Errorf("SMTP DATA 失败: %w", err)
	}
	if _, err := writer.Write(raw); err != nil {
		_ = writer.Close()
		return fmt.Errorf("SMTP 写入邮件失败: %w", err)
	}
	if err := writer.Close(); err != nil {
		return fmt.Errorf("SMTP 提交邮件失败: %w", err)
	}
	_ = client.Quit()
	return nil
}

func smtpEnvelopeRecipients(payload SendMessagePayload) []string {
	seen := map[string]bool{}
	recipients := []string{}
	for _, candidate := range append(append(nonNilStrings(payload.Recipients), nonNilStrings(payload.CC)...), nonNilStrings(payload.BCC)...) {
		candidate = strings.TrimSpace(candidate)
		key := strings.ToLower(candidate)
		if candidate == "" || seen[key] {
			continue
		}
		seen[key] = true
		recipients = append(recipients, candidate)
	}
	return recipients
}

func buildSMTPMessage(user UserRecord, message MailMessage, payload SendMessagePayload) ([]byte, error) {
	var out bytes.Buffer
	from := netmail.Address{Name: firstNonEmpty(user.DisplayName, user.Email), Address: user.Email}
	headers := textproto.MIMEHeader{}
	headers.Set("From", from.String())
	headers.Set("To", strings.Join(payload.Recipients, ", "))
	if len(payload.CC) > 0 {
		headers.Set("Cc", strings.Join(payload.CC, ", "))
	}
	headers.Set("Subject", mime.QEncoding.Encode("utf-8", payload.Subject))
	headers.Set("Date", time.Now().Format(time.RFC1123Z))
	headers.Set("Message-ID", "<"+message.ID+"@"+fallbackDomain(user.Email)+">")
	headers.Set("MIME-Version", "1.0")
	attachments := payload.Attachments
	if len(attachments) == 0 {
		contentType := "text/plain; charset=utf-8"
		body := payload.Body.Text
		if payload.Body.Format == "html" || (payload.Body.HTML != "" && payload.Body.Text == "") {
			contentType = "text/html; charset=utf-8"
			body = payload.Body.HTML
		}
		headers.Set("Content-Type", contentType)
		headers.Set("Content-Transfer-Encoding", "8bit")
		writeMIMEHeaders(&out, headers)
		out.WriteString("\r\n")
		out.WriteString(firstNonEmpty(body, stripHTML(message.Content), payload.Subject))
		return out.Bytes(), nil
	}
	mixed := multipart.NewWriter(&out)
	headers.Set("Content-Type", `multipart/mixed; boundary="`+mixed.Boundary()+`"`)
	writeMIMEHeaders(&out, headers)
	out.WriteString("\r\n")
	bodyHeader := textproto.MIMEHeader{}
	bodyHeader.Set("Content-Type", "text/plain; charset=utf-8")
	body := payload.Body.Text
	if payload.Body.Format == "html" || payload.Body.HTML != "" {
		bodyHeader.Set("Content-Type", "text/html; charset=utf-8")
		body = firstNonEmpty(payload.Body.HTML, payload.Body.Text)
	}
	bodyHeader.Set("Content-Transfer-Encoding", "8bit")
	bodyPart, err := mixed.CreatePart(bodyHeader)
	if err != nil {
		return nil, fmt.Errorf("创建邮件正文失败: %w", err)
	}
	if _, err := bodyPart.Write([]byte(firstNonEmpty(body, payload.Subject))); err != nil {
		return nil, fmt.Errorf("写入邮件正文失败: %w", err)
	}
	for _, attachment := range attachments {
		source, err := attachmentMap(attachment)
		if err != nil {
			return nil, err
		}
		contentBase64 := strings.TrimSpace(firstNonEmpty(stringFromAny(source["contentBase64"]), stringFromAny(source["content_base64"])))
		if contentBase64 == "" {
			continue
		}
		decoded, err := base64.StdEncoding.DecodeString(contentBase64)
		if err != nil {
			return nil, fmt.Errorf("附件 %s 内容不是有效 base64", firstNonEmpty(stringFromAny(source["name"]), "未命名附件"))
		}
		name := firstNonEmpty(stringFromAny(source["name"]), stringFromAny(source["filename"]), "attachment")
		contentType := firstNonEmpty(stringFromAny(source["contentType"]), stringFromAny(source["content_type"]), "application/octet-stream")
		partHeader := textproto.MIMEHeader{}
		partHeader.Set("Content-Type", contentType+`; name="`+strings.ReplaceAll(name, `"`, `'`)+`"`)
		partHeader.Set("Content-Disposition", `attachment; filename="`+strings.ReplaceAll(name, `"`, `'`)+`"; filename*=UTF-8''`+url.PathEscape(name))
		partHeader.Set("Content-Transfer-Encoding", "base64")
		part, err := mixed.CreatePart(partHeader)
		if err != nil {
			return nil, fmt.Errorf("创建附件失败: %w", err)
		}
		if _, err := part.Write([]byte(wrapBase64(base64.StdEncoding.EncodeToString(decoded)))); err != nil {
			return nil, fmt.Errorf("写入附件失败: %w", err)
		}
	}
	if err := mixed.Close(); err != nil {
		return nil, fmt.Errorf("结束 MIME 邮件失败: %w", err)
	}
	return out.Bytes(), nil
}

func writeMIMEHeaders(out *bytes.Buffer, headers textproto.MIMEHeader) {
	keys := make([]string, 0, len(headers))
	for key := range headers {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		for _, value := range headers[key] {
			out.WriteString(key)
			out.WriteString(": ")
			out.WriteString(value)
			out.WriteString("\r\n")
		}
	}
}

func wrapBase64(value string) string {
	if len(value) <= 76 {
		return value + "\r\n"
	}
	var builder strings.Builder
	for len(value) > 76 {
		builder.WriteString(value[:76])
		builder.WriteString("\r\n")
		value = value[76:]
	}
	if value != "" {
		builder.WriteString(value)
		builder.WriteString("\r\n")
	}
	return builder.String()
}

func parseMailboxMessageRelayResult(raw []byte) MailboxMessageRelayResult {
	if len(raw) == 0 {
		return MailboxMessageRelayResult{}
	}
	payload := unwrapEnvelopeData(raw)
	var result MailboxMessageRelayResult
	_ = json.Unmarshal(payload, &result)
	var generic map[string]any
	if err := json.Unmarshal(payload, &generic); err == nil {
		result.AcceptedAt = firstNonEmpty(result.AcceptedAt, stringFromAny(generic["acceptedAt"]), stringFromAny(generic["accepted_at"]))
		result.ProviderMessageID = firstNonEmpty(
			result.ProviderMessageID,
			stringFromAny(generic["providerMessageId"]),
			stringFromAny(generic["provider_message_id"]),
			stringFromAny(generic["messageId"]),
			stringFromAny(generic["message_id"]),
		)
		result.Status = firstNonEmpty(result.Status, stringFromAny(generic["status"]))
		result.MessageText = firstNonEmpty(result.MessageText, stringFromAny(generic["message"]), stringFromAny(generic["messageText"]))
	}
	return result
}

func unwrapEnvelopeData(raw []byte) []byte {
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return raw
	}
	if data, ok := envelope["data"]; ok && len(data) > 0 && string(data) != "null" {
		return data
	}
	return raw
}

func splitEmailAddress(email string) (string, string) {
	email = strings.TrimSpace(strings.ToLower(email))
	parts := strings.Split(email, "@")
	if len(parts) != 2 {
		return "", ""
	}
	return strings.TrimSpace(parts[0]), normalizeDomain(parts[1])
}

func randomMailboxPassword() string {
	return randomBase32(32)
}

func rememberMailboxCredential(user *UserRecord, provisioned MailboxProvisionResult) error {
	if user == nil {
		return nil
	}
	user.MailboxUsername = firstNonEmpty(strings.TrimSpace(provisioned.MailboxUsername), strings.TrimSpace(user.MailboxUsername), user.Email)
	password := strings.TrimSpace(provisioned.MailboxPassword)
	if password == "" {
		return nil
	}
	secret, err := sealMailboxPassword(password)
	if err != nil {
		return err
	}
	user.MailboxPasswordSecret = secret
	return nil
}

func sealMailboxPassword(password string) (string, error) {
	password = strings.TrimSpace(password)
	if password == "" {
		return "", nil
	}
	keyMaterial := strings.TrimSpace(env("MAILBOX_CREDENTIAL_KEY", ""))
	if keyMaterial == "" {
		if productionStrictEnabled() {
			return "", errors.New("生产严格模式必须配置 MAILBOX_CREDENTIAL_KEY，禁止明文保存邮箱密码")
		}
		return "plain:" + base64.StdEncoding.EncodeToString([]byte(password)), nil
	}
	key := sha256.Sum256([]byte(keyMaterial))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return "", fmt.Errorf("初始化邮箱凭据加密失败: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("初始化邮箱凭据封装失败: %w", err)
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", fmt.Errorf("生成邮箱凭据随机数失败: %w", err)
	}
	sealed := gcm.Seal(nonce, nonce, []byte(password), nil)
	return "v1:" + base64.RawStdEncoding.EncodeToString(sealed), nil
}

func openMailboxPasswordSecret(secret string) (string, error) {
	secret = strings.TrimSpace(secret)
	if secret == "" {
		return "", nil
	}
	if strings.HasPrefix(secret, "plain:") {
		raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(secret, "plain:"))
		if err != nil {
			return "", fmt.Errorf("邮箱凭据解析失败: %w", err)
		}
		return string(raw), nil
	}
	if strings.HasPrefix(secret, "v1:") {
		keyMaterial := strings.TrimSpace(env("MAILBOX_CREDENTIAL_KEY", ""))
		if keyMaterial == "" {
			return "", errors.New("邮箱凭据需要配置 MAILBOX_CREDENTIAL_KEY 才能解密")
		}
		raw, err := base64.RawStdEncoding.DecodeString(strings.TrimPrefix(secret, "v1:"))
		if err != nil {
			return "", fmt.Errorf("邮箱凭据解析失败: %w", err)
		}
		key := sha256.Sum256([]byte(keyMaterial))
		block, err := aes.NewCipher(key[:])
		if err != nil {
			return "", fmt.Errorf("初始化邮箱凭据解密失败: %w", err)
		}
		gcm, err := cipher.NewGCM(block)
		if err != nil {
			return "", fmt.Errorf("初始化邮箱凭据解封失败: %w", err)
		}
		if len(raw) < gcm.NonceSize() {
			return "", errors.New("邮箱凭据密文不完整")
		}
		nonce := raw[:gcm.NonceSize()]
		ciphertext := raw[gcm.NonceSize():]
		opened, err := gcm.Open(nil, nonce, ciphertext, nil)
		if err != nil {
			return "", fmt.Errorf("邮箱凭据解密失败: %w", err)
		}
		return string(opened), nil
	}
	return secret, nil
}

func provisionRetryDelay(attempts int) time.Duration {
	if attempts < 1 {
		attempts = 1
	}
	if attempts > 6 {
		attempts = 6
	}
	return time.Duration(attempts*attempts) * time.Minute
}

func processProvisionJobs(ctx context.Context, state *AppState, limit int, now time.Time) ProvisionRunSummary {
	summary := ProvisionRunSummary{Message: "开通队列暂无待处理任务"}
	if limit <= 0 {
		limit = 25
	}
	server := normalizeMailServerConfig(state.Config.Mailbox.Server)
	nowISOValue := now.Format(time.RFC3339)
	for index := range state.Config.ProvisionJobs {
		if summary.Processed >= limit {
			break
		}
		job := &state.Config.ProvisionJobs[index]
		*job = normalizeProvisionJob(*job)
		if job.Status == "succeeded" || job.Status == "running" {
			summary.Skipped += 1
			continue
		}
		if job.NextRunAt != "" {
			nextRunAt, err := time.Parse(time.RFC3339, job.NextRunAt)
			if err == nil && now.Before(nextRunAt) {
				summary.Skipped += 1
				continue
			}
		}
		userIndex, ok := findUserIndexByID(state.Users, job.AccountID)
		if !ok {
			job.Status = "failed"
			job.LastError = "账号不存在"
			job.Attempts += 1
			job.LastRunAt = nowISOValue
			job.UpdatedAt = nowISOValue
			summary.Processed += 1
			summary.Failed += 1
			continue
		}
		user := &state.Users[userIndex]
		if !accountActive(user.RegisteredUser) {
			job.Status = "blocked"
			job.LastError = "账号已禁用"
			job.LastRunAt = nowISOValue
			job.UpdatedAt = nowISOValue
			updateRegisteredUser(state.Config.RegisteredUsers, user.RegisteredUser)
			summary.Processed += 1
			summary.Failed += 1
			continue
		}
		job.Attempts += 1
		job.LastRunAt = nowISOValue
		job.UpdatedAt = nowISOValue
		summary.Processed += 1
		if !server.Enabled || server.BaseURL == "" {
			job.Status = "blocked"
			job.LastError = "邮件服务未配置"
			user.MailboxStatus = "pending_config"
			user.MailboxLastError = job.LastError
			user.MailboxProvisionedAt = ""
			updateRegisteredUser(state.Config.RegisteredUsers, user.RegisteredUser)
			summary.Failed += 1
			continue
		}
		if server.Status != "online" {
			job.Status = "failed"
			job.LastError = firstNonEmpty(server.LastError, "邮件服务未连通")
			user.MailboxStatus = "failed"
			user.MailboxLastError = job.LastError
			user.MailboxProvisionedAt = ""
			updateRegisteredUser(state.Config.RegisteredUsers, user.RegisteredUser)
			summary.Failed += 1
			continue
		}
		provisioned, err := provisionMailboxAccount(ctx, state.Config, *user, *job)
		state.Config.Mailbox.Server.LastProvisionCheckAt = nowISOValue
		if err != nil {
			job.Status = "failed"
			job.LastError = err.Error()
			job.NextRunAt = now.Add(provisionRetryDelay(job.Attempts)).Format(time.RFC3339)
			user.MailboxStatus = "failed"
			user.MailboxLastError = job.LastError
			user.MailboxProvisionedAt = ""
			updateRegisteredUser(state.Config.RegisteredUsers, user.RegisteredUser)
			summary.Failed += 1
			continue
		}
		job.Status = "succeeded"
		job.LastError = ""
		job.CompletedAt = nowISOValue
		job.NextRunAt = ""
		user.MailboxStatus = "provisioned"
		user.MailboxProvisionedAt = nowISOValue
		user.MailboxExternalID = firstNonEmpty(provisioned.ExternalID, user.MailboxExternalID, "stalwart_"+user.ID)
		user.MailboxLastError = ""
		if err := rememberMailboxCredential(user, provisioned); err != nil {
			job.Status = "failed"
			job.LastError = err.Error()
			job.NextRunAt = now.Add(provisionRetryDelay(job.Attempts)).Format(time.RFC3339)
			job.CompletedAt = ""
			user.MailboxStatus = "failed"
			user.MailboxLastError = job.LastError
			user.MailboxProvisionedAt = ""
			updateRegisteredUser(state.Config.RegisteredUsers, user.RegisteredUser)
			summary.Failed += 1
			continue
		}
		updateRegisteredUser(state.Config.RegisteredUsers, user.RegisteredUser)
		summary.Succeeded += 1
	}
	if summary.Processed > 0 {
		summary.Message = fmt.Sprintf("已处理 %d 个邮箱开通任务，成功 %d 个，失败 %d 个", summary.Processed, summary.Succeeded, summary.Failed)
	}
	return summary
}

func ensureProvisionJobsForAccounts(state *AppState, now string) {
	for _, user := range state.Users {
		if !accountActive(user.RegisteredUser) || normalizeMailboxStatus(user.MailboxStatus) == "provisioned" {
			continue
		}
		if findOpenProvisionJob(state.Config.ProvisionJobs, user.ID) != nil {
			continue
		}
		queueProvisionJob(state, user, now)
	}
}

func findOpenProvisionJob(jobs []ProvisionJob, accountID string) *ProvisionJob {
	for index := range jobs {
		if jobs[index].AccountID != accountID {
			continue
		}
		if normalizeProvisionJobStatus(jobs[index].Status) != "succeeded" {
			return &jobs[index]
		}
	}
	return nil
}

func resolveSMTPCredentials(config MailServerConfig, user UserRecord) (string, string, bool, error) {
	usernameTemplate := strings.TrimSpace(config.SMTPUsername)
	password := strings.TrimSpace(config.SMTPPassword)
	if usernameTemplate == "" && password == "" {
		return "", "", false, nil
	}
	username := renderMailboxCredentialTemplate(usernameTemplate, user)
	if username == "" {
		username = firstNonEmpty(user.MailboxUsername, user.Email)
	}
	if password == "" {
		var err error
		password, err = openMailboxPasswordSecret(user.MailboxPasswordSecret)
		if err != nil {
			return "", "", false, err
		}
	}
	if strings.TrimSpace(username) == "" {
		return "", "", false, errors.New("SMTP 账号未配置，可使用 {email}、{localPart}、{domain}、{mailboxUsername} 模板")
	}
	if password == "" {
		return "", "", false, errors.New("SMTP 密码未配置，可配置 SMTP 主密码，或先完成邮箱开通以保存用户邮箱凭据")
	}
	return username, password, true, nil
}

func provisionJobCounts(jobs []ProvisionJob) (queued int, failed int, completed int) {
	for _, job := range jobs {
		switch normalizeProvisionJobStatus(job.Status) {
		case "queued", "running":
			queued += 1
		case "failed", "blocked":
			failed += 1
		case "succeeded":
			completed += 1
		}
	}
	return queued, failed, completed
}

func activeRegisteredSeats(users []UserRecord) int {
	count := 0
	for _, user := range users {
		if normalizeAccountStatus(user.Status) != "disabled" {
			count += 1
		}
	}
	return count
}

func activeInviteReservations(invites []InviteRecord) int {
	count := 0
	for _, invite := range invites {
		if inviteActive(invite) {
			count += 1
		}
	}
	return count
}

func buildUsageSnapshot(state AppState) UsageSnapshot {
	activeSeats := activeRegisteredSeats(state.Users)
	reservedSeats := activeInviteReservations(state.Config.Invites)
	usedBytes := storageUsedBytes(state.Messages)
	usedMB := int64(0)
	if usedBytes > 0 {
		usedMB = (usedBytes + 1024*1024 - 1) / (1024 * 1024)
	}
	return UsageSnapshot{
		ActiveSeats:      activeSeats,
		ReservedSeats:    reservedSeats,
		UsedSeats:        activeSeats + reservedSeats,
		StorageUsedBytes: usedBytes,
		StorageUsedMB:    usedMB,
		UpdatedAt:        state.Config.UpdatedAt,
	}
}

func storageUsedBytes(messages map[string][]MailMessage) int64 {
	var total int64
	for _, items := range messages {
		total += messagesSizeBytes(items)
	}
	return total
}

func messagesSizeBytes(messages []MailMessage) int64 {
	var total int64
	for _, message := range messages {
		total += int64(len(message.ID) + len(message.ThreadID) + len(message.Folder) + len(message.Sender) + len(message.SenderEmail))
		total += int64(len(strings.Join(message.Recipients, ",")) + len(message.Subject) + len(message.Snippet) + len(message.Content))
		total += int64(len(strings.Join(message.Tags, ",")) + len(message.DeliveryStatus) + len(message.Source))
		for _, attachment := range message.Attachments {
			total += attachmentSizeBytes(attachment)
		}
	}
	return total
}

func attachmentSizeBytes(attachment any) int64 {
	switch value := attachment.(type) {
	case map[string]any:
		for _, key := range []string{"sizeBytes", "size", "bytes"} {
			if number, ok := numericAnyToInt64(value[key]); ok && number > 0 {
				return number
			}
		}
	case map[string]string:
		for _, key := range []string{"sizeBytes", "size", "bytes"} {
			if number, err := strconv.ParseInt(strings.TrimSpace(value[key]), 10, 64); err == nil && number > 0 {
				return number
			}
		}
	}
	raw, err := json.Marshal(attachment)
	if err != nil {
		return 0
	}
	return int64(len(raw))
}

func numericAnyToInt64(value any) (int64, bool) {
	switch number := value.(type) {
	case int:
		return int64(number), true
	case int64:
		return number, true
	case int32:
		return int64(number), true
	case float64:
		return int64(number), true
	case float32:
		return int64(number), true
	case json.Number:
		parsed, err := number.Int64()
		return parsed, err == nil
	case string:
		parsed, err := strconv.ParseInt(strings.TrimSpace(number), 10, 64)
		return parsed, err == nil
	default:
		return 0, false
	}
}

func normalizeMailServerConfig(config MailServerConfig) MailServerConfig {
	if strings.TrimSpace(config.Provider) == "" {
		config.Provider = "stalwart"
	}
	config.BaseURL = strings.TrimRight(strings.TrimSpace(config.BaseURL), "/")
	config.ProvisionPath = normalizeProvisionPath(config.ProvisionPath)
	config.LifecyclePath = normalizeProvisionPath(config.LifecyclePath)
	config.MessageListPath = normalizeProvisionPath(config.MessageListPath)
	config.MessageDetailPath = normalizeProvisionPath(config.MessageDetailPath)
	config.MessageSendPath = normalizeProvisionPath(config.MessageSendPath)
	config.DraftPath = normalizeProvisionPath(config.DraftPath)
	config.MessageStarPath = normalizeProvisionPath(config.MessageStarPath)
	config.MessageMovePath = normalizeProvisionPath(config.MessageMovePath)
	config.MessageReadPath = normalizeProvisionPath(config.MessageReadPath)
	config.AdminToken = strings.TrimSpace(config.AdminToken)
	config.SMTPHost = strings.TrimSpace(config.SMTPHost)
	if config.SMTPPort <= 0 || config.SMTPPort > 65535 {
		config.SMTPPort = 25
	}
	config.SMTPUsername = strings.TrimSpace(config.SMTPUsername)
	config.SMTPPassword = strings.TrimSpace(config.SMTPPassword)
	config.SMTPTLSMode = strings.ToLower(strings.TrimSpace(config.SMTPTLSMode))
	switch config.SMTPTLSMode {
	case "none", "starttls", "auto", "tls":
	default:
		config.SMTPTLSMode = "auto"
	}
	if config.AdminToken != "" {
		config.AdminTokenSet = true
		config.AdminTokenMasked = maskSecret(config.AdminToken)
	}
	if config.SMTPPassword != "" {
		config.SMTPPasswordSet = true
		config.SMTPPasswordMasked = maskSecret(config.SMTPPassword)
	}
	config.IMAPHost = strings.TrimSpace(config.IMAPHost)
	if config.IMAPPort <= 0 || config.IMAPPort > 65535 {
		config.IMAPPort = 993
	}
	config.IMAPUsername = strings.TrimSpace(config.IMAPUsername)
	config.IMAPPassword = strings.TrimSpace(config.IMAPPassword)
	config.IMAPTLSMode = strings.ToLower(strings.TrimSpace(config.IMAPTLSMode))
	switch config.IMAPTLSMode {
	case "none", "starttls", "auto", "tls":
	default:
		config.IMAPTLSMode = "tls"
	}
	if config.IMAPPassword != "" {
		config.IMAPPasswordSet = true
		config.IMAPPasswordMasked = maskSecret(config.IMAPPassword)
	}
	if !config.Enabled && !smtpOutboundReady(config) && !imapInboundReady(config) {
		config.Status = firstNonEmpty(config.Status, "not_configured")
	} else if config.BaseURL == "" && !smtpOutboundReady(config) && !imapInboundReady(config) {
		config.Status = firstNonEmpty(config.Status, "not_configured")
	} else {
		config.Status = firstNonEmpty(config.Status, "unknown")
	}
	if !config.AdminTokenSet {
		config.AdminTokenMasked = ""
	}
	if !config.SMTPPasswordSet {
		config.SMTPPasswordMasked = ""
	}
	if !config.IMAPPasswordSet {
		config.IMAPPasswordMasked = ""
	}
	return config
}

func normalizeProvisionPath(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if strings.HasPrefix(value, "http://") || strings.HasPrefix(value, "https://") {
		return strings.TrimRight(value, "/")
	}
	if !strings.HasPrefix(value, "/") {
		value = "/" + value
	}
	return value
}

func testMailServerConnection(ctx context.Context, config MailServerConfig) MailServerConfig {
	config = normalizeMailServerConfig(config)
	config.LastCheckedAt = nowISO()
	config.LastError = ""
	if !config.Enabled {
		if !smtpOutboundReady(config) && !imapInboundReady(config) {
			config.Status = "not_configured"
			config.LastError = "邮件服务未启用"
			return config
		}
	}
	checkErrors := []string{}
	checkedNative := false
	if smtpOutboundReady(config) {
		checkedNative = true
		if err := testSMTPConnection(ctx, config); err == nil {
		} else {
			checkErrors = append(checkErrors, "SMTP: "+err.Error())
		}
	}
	if imapInboundReady(config) {
		checkedNative = true
		var err error
		if imapConnectionTestNeedsOnlyProtocol(config) {
			err = testIMAPProtocolConnection(ctx, config)
		} else {
			err = testIMAPConnection(ctx, config, UserRecord{})
		}
		if err != nil {
			checkErrors = append(checkErrors, "IMAP: "+err.Error())
		}
	}
	if checkedNative && len(checkErrors) == 0 && config.BaseURL == "" {
		config.Status = "online"
		return config
	}
	if checkedNative && len(checkErrors) > 0 && config.BaseURL == "" {
		config.Status = "offline"
		config.LastError = strings.Join(checkErrors, "；")
		return config
	}
	if len(checkErrors) > 0 {
		config.LastError = strings.Join(checkErrors, "；")
	}
	if config.BaseURL == "" {
		config.Status = "not_configured"
		config.LastError = "请先配置邮件服务地址"
		return config
	}
	ctx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, config.BaseURL, nil)
	if err != nil {
		config.Status = "offline"
		config.LastError = "邮件服务地址无效"
		return config
	}
	applyMailServerAuthHeaders(req, config)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		config.Status = "offline"
		config.LastError = err.Error()
		return config
	}
	defer res.Body.Close()
	if res.StatusCode >= 200 && res.StatusCode < 500 {
		config.Status = "online"
		return config
	}
	config.Status = "offline"
	config.LastError = fmt.Sprintf("邮件服务返回 HTTP %d", res.StatusCode)
	return config
}

func testSMTPConnection(ctx context.Context, config MailServerConfig) error {
	config = normalizeMailServerConfig(config)
	if !smtpOutboundReady(config) {
		return errors.New("SMTP 发信未配置")
	}
	ctx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()
	dialer := net.Dialer{}
	address := net.JoinHostPort(config.SMTPHost, strconv.Itoa(config.SMTPPort))
	var conn net.Conn
	var err error
	if config.SMTPTLSMode == "tls" {
		tlsDialer := tls.Dialer{NetDialer: &dialer, Config: &tls.Config{ServerName: config.SMTPHost, MinVersion: tls.VersionTLS12}}
		conn, err = tlsDialer.DialContext(ctx, "tcp", address)
	} else {
		conn, err = dialer.DialContext(ctx, "tcp", address)
	}
	if err != nil {
		return fmt.Errorf("SMTP 连接失败: %w", err)
	}
	client, err := smtp.NewClient(conn, config.SMTPHost)
	if err != nil {
		_ = conn.Close()
		return fmt.Errorf("SMTP 握手失败: %w", err)
	}
	defer client.Close()
	if err := client.Hello("infinitemail"); err != nil {
		return fmt.Errorf("SMTP HELO 失败: %w", err)
	}
	if config.SMTPTLSMode == "starttls" {
		if ok, _ := client.Extension("STARTTLS"); !ok {
			return errors.New("SMTP 服务未提供 STARTTLS")
		}
	}
	if config.SMTPUsername != "" || config.SMTPPassword != "" {
		if ok, _ := client.Extension("AUTH"); !ok {
			return errors.New("SMTP 服务未提供 AUTH")
		}
	}
	_ = client.Quit()
	return nil
}

func applyMailboxProvisionState(user *UserRecord, server MailServerConfig) {
	server = normalizeMailServerConfig(server)
	user.MailboxExternalID = firstNonEmpty(user.MailboxExternalID, user.ID)
	if !server.Enabled || server.BaseURL == "" {
		user.MailboxStatus = "pending_config"
		user.MailboxLastError = "邮件服务未配置"
		user.MailboxProvisionedAt = ""
		return
	}
	if server.Status == "online" {
		user.MailboxStatus = "queued"
		user.MailboxLastError = "等待邮件服务开通任务执行"
		user.MailboxProvisionedAt = ""
		return
	}
	user.MailboxStatus = "failed"
	user.MailboxLastError = firstNonEmpty(server.LastError, "邮件服务未连通")
	user.MailboxProvisionedAt = ""
}

func mailboxProvisionMessage(user *UserRecord) string {
	if user == nil {
		return "邮箱状态未知"
	}
	switch normalizeMailboxStatus(user.MailboxStatus) {
	case "provisioned":
		return "邮箱已真实开通"
	case "queued":
		return "邮箱开通任务已进入队列"
	case "failed":
		return firstNonEmpty(user.MailboxLastError, "邮箱开通失败，请联系管理员")
	default:
		return firstNonEmpty(user.MailboxLastError, "邮件服务未配置，邮箱待后台开通")
	}
}

func ensureProvisionedMailbox(user UserRecord) error {
	if strings.TrimSpace(user.Email) == "" {
		return errors.New("请先设置邮箱地址")
	}
	if normalizeMailboxStatus(user.MailboxStatus) != "provisioned" {
		return fmt.Errorf("邮箱尚未真实开通：%s", mailboxProvisionMessage(&user))
	}
	return nil
}

func buildMailboxLocalPart(config MailboxConfig, emailPrefix string, prefix string) (string, error) {
	localName := normalizeMailboxName(emailPrefix)
	if localName == "" {
		return "", errors.New("请输入邮箱名")
	}
	if !config.PrefixPolicyEnabled {
		return localName, nil
	}
	allowed := normalizeAllowedPrefixes(config.AllowedPrefixes)
	selectedPrefix := normalizePrefix(prefix)
	if selectedPrefix == "" {
		selectedPrefix = resolveDefaultPrefix(config)
	}
	if !contains(allowed, selectedPrefix) {
		return "", errors.New("当前前缀未开放注册")
	}
	localName = strings.TrimPrefix(localName, selectedPrefix+"-")
	return selectedPrefix + "-" + localName, nil
}

func defaultDNSCheck(domain string) DNSCheck {
	domain = normalizeDomain(domain)
	return DNSCheck{
		Status:      "pending",
		Domain:      domain,
		Selector:    "infinitemail",
		Recommended: recommendedDNSRecords(domain),
		Records:     []DNSRecordStatus{},
	}
}

func normalizeDNSCheck(domain string, check DNSCheck) DNSCheck {
	domain = normalizeDomain(domain)
	if check.Domain == "" || !strings.EqualFold(check.Domain, domain) {
		check = defaultDNSCheck(domain)
	}
	check.Domain = domain
	if strings.TrimSpace(check.Selector) == "" {
		check.Selector = "infinitemail"
	}
	if check.Status == "" {
		check.Status = "pending"
	}
	check.Recommended = recommendedDNSRecords(domain)
	if check.Records == nil {
		check.Records = []DNSRecordStatus{}
	}
	check.TotalRecords = len(check.Recommended)
	if check.TotalRecords == 0 {
		check.TotalRecords = 4
	}
	verified := 0
	for _, record := range check.Records {
		if record.Verified {
			verified += 1
		}
	}
	check.VerifiedRecords = verified
	return check
}

func recommendedDNSRecords(domain string) []DNSRecordStatus {
	domain = normalizeDomain(domain)
	return []DNSRecordStatus{
		{Type: "MX", Host: "@", Expected: "10 mail." + domain},
		{Type: "TXT", Host: "@", Expected: "v=spf1 mx ~all"},
		{Type: "TXT", Host: "_dmarc", Expected: "v=DMARC1; p=quarantine; rua=mailto:dmarc@" + domain},
		{Type: "TXT", Host: "infinitemail._domainkey", Expected: "v=DKIM1; k=rsa; p=<Stalwart DKIM public key>"},
	}
}

func verifyDomainDNS(ctx context.Context, domain string) DNSCheck {
	domain = normalizeDomain(domain)
	ctx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()

	check := defaultDNSCheck(domain)
	check.CheckedAt = nowISO()
	check.NextCheckAfter = time.Now().Add(10 * time.Minute).Format(time.RFC3339)
	records := []DNSRecordStatus{
		verifyMXRecord(ctx, domain),
		verifySPFRecord(ctx, domain),
		verifyDMARCRecord(ctx, domain),
		verifyDKIMRecord(ctx, domain, check.Selector),
	}
	check.Records = records
	check.TotalRecords = len(records)
	for _, record := range records {
		if record.Verified {
			check.VerifiedRecords += 1
		}
	}
	if check.VerifiedRecords == check.TotalRecords {
		check.Status = "verified"
		check.VerifiedAt = check.CheckedAt
		check.NextCheckAfter = ""
	} else if check.VerifiedRecords > 0 {
		check.Status = "partial"
	} else {
		check.Status = "pending"
	}
	return check
}

func verifyMXRecord(ctx context.Context, domain string) DNSRecordStatus {
	record := DNSRecordStatus{Type: "MX", Host: "@", Expected: "10 mail." + domain}
	mxs, err := net.DefaultResolver.LookupMX(ctx, domain)
	if err != nil || len(mxs) == 0 {
		record.Message = "未查询到 MX 记录"
		return record
	}
	actual := make([]string, 0, len(mxs))
	for _, mx := range mxs {
		actual = append(actual, fmt.Sprintf("%d %s", mx.Pref, strings.TrimSuffix(mx.Host, ".")))
	}
	sort.Strings(actual)
	record.Actual = strings.Join(actual, "; ")
	record.Verified = true
	record.Message = "已检测到 MX 记录"
	return record
}

func verifySPFRecord(ctx context.Context, domain string) DNSRecordStatus {
	record := DNSRecordStatus{Type: "TXT", Host: "@", Expected: "v=spf1 mx ~all"}
	txts, err := net.DefaultResolver.LookupTXT(ctx, domain)
	if err != nil {
		record.Message = "未查询到 SPF TXT 记录"
		return record
	}
	for _, txt := range txts {
		normalized := strings.ToLower(strings.TrimSpace(txt))
		if strings.HasPrefix(normalized, "v=spf1") {
			record.Actual = txt
			record.Verified = strings.Contains(normalized, " mx") || strings.Contains(normalized, " include:") || strings.Contains(normalized, " ip4:") || strings.Contains(normalized, " ip6:") || strings.Contains(normalized, " a")
			if record.Verified {
				record.Message = "SPF 已配置"
			} else {
				record.Message = "SPF 存在，但未包含可投递来源"
			}
			return record
		}
	}
	record.Message = "未查询到 SPF TXT 记录"
	return record
}

func verifyDMARCRecord(ctx context.Context, domain string) DNSRecordStatus {
	record := DNSRecordStatus{Type: "TXT", Host: "_dmarc", Expected: "v=DMARC1; p=quarantine; rua=mailto:dmarc@" + domain}
	txts, err := net.DefaultResolver.LookupTXT(ctx, "_dmarc."+domain)
	if err != nil {
		record.Message = "未查询到 DMARC 记录"
		return record
	}
	for _, txt := range txts {
		if strings.HasPrefix(strings.ToLower(strings.TrimSpace(txt)), "v=dmarc1") {
			record.Actual = txt
			record.Verified = true
			record.Message = "DMARC 已配置"
			return record
		}
	}
	record.Message = "未查询到 DMARC 记录"
	return record
}

func verifyDKIMRecord(ctx context.Context, domain string, selector string) DNSRecordStatus {
	selector = firstNonEmpty(strings.TrimSpace(selector), "infinitemail")
	host := selector + "._domainkey"
	record := DNSRecordStatus{Type: "TXT", Host: host, Expected: "v=DKIM1; k=rsa; p=<Stalwart DKIM public key>"}
	txts, err := net.DefaultResolver.LookupTXT(ctx, host+"."+domain)
	if err != nil {
		record.Message = "未查询到 DKIM 记录"
		return record
	}
	for _, txt := range txts {
		if strings.Contains(strings.ToLower(txt), "v=dkim1") {
			record.Actual = txt
			record.Verified = true
			record.Message = "DKIM 已配置"
			return record
		}
	}
	record.Message = "未查询到 DKIM 记录"
	return record
}

func buildProfile(user UserRecord, mailbox MailboxConfig) MailProfile {
	localPart := strings.Split(user.Email, "@")[0]
	prefix := ""
	emailPrefix := localPart
	if mailbox.PrefixPolicyEnabled && strings.Contains(localPart, "-") {
		parts := strings.SplitN(localPart, "-", 2)
		prefix = parts[0] + "-"
		emailPrefix = parts[1]
	}
	status := normalizeMailboxStatus(user.MailboxStatus)
	hasEmail := strings.TrimSpace(user.Email) != ""
	mailboxProvisioned := hasEmail && status == "provisioned"
	provisionedAt := ""
	if mailboxProvisioned {
		provisionedAt = user.MailboxProvisionedAt
	}
	return MailProfile{
		ID:                  user.ID,
		DisplayName:         firstNonEmpty(user.DisplayName, "MyName"),
		AvatarInitial:       strings.ToUpper(firstRune(firstNonEmpty(user.DisplayName, user.Phone, "M"))),
		UnifiedAccountPhone: maskPhone(user.Phone),
		RolePrefix:          prefix,
		EmailPrefix:         emailPrefix,
		Email:               user.Email,
		MailboxDomain:       mailbox.Domain,
		MailboxProvisioned:  mailboxProvisioned,
		ProvisioningStatus:  status,
		AuthMode:            "user",
		SourceUserID:        user.ID,
		CreatedAt:           user.RegisteredAt,
		ProvisionedAt:       provisionedAt,
	}
}

func sessionPayload(token string, session SessionRecord, user UserRecord, config AdminConfig) response {
	profile := buildProfile(user, config.Mailbox)
	hasEmail := strings.TrimSpace(profile.Email) != ""
	if token == "" {
		token = session.Token
	}
	return response{
		"token":                token,
		"expiresIn":            int(sessionMaxAge.Seconds()),
		"isAuthenticated":      true,
		"requiresActivation":   !hasEmail,
		"requiresProvisioning": hasEmail && !profile.MailboxProvisioned,
		"mailboxProvisioned":   profile.MailboxProvisioned,
		"provisioningStatus":   profile.ProvisioningStatus,
		"rolePrefix":           profile.RolePrefix,
		"profile":              profile,
		"user": response{
			"id":       user.ID,
			"phone":    user.Phone,
			"name":     user.DisplayName,
			"nickname": user.DisplayName,
			"authMode": "user",
		},
		"adminConfig": publicAdminConfig(config),
	}
}

func createSession(userID string) SessionRecord {
	now := time.Now()
	token := "bff_" + randomHex(32)
	return SessionRecord{
		ID:         "sess_" + randomHex(12),
		Token:      token,
		UserID:     userID,
		CreatedAt:  now.Format(time.RFC3339),
		LastSeenAt: now.Format(time.RFC3339),
		ExpiresAt:  now.Add(sessionMaxAge).Format(time.RFC3339),
	}
}

func createSessionForRequest(userID string, r *http.Request) SessionRecord {
	session := createSession(userID)
	updateSessionRequestMetadata(&session, r, session.CreatedAt)
	return session
}

func putSession(sessions map[string]SessionRecord, session SessionRecord) {
	if sessions == nil || session.Token == "" {
		return
	}
	key := sessionStorageKey(session.Token)
	sessions[key] = normalizeSessionRecord(key, session)
}

func lookupSession(sessions map[string]SessionRecord, token string) (SessionRecord, bool) {
	token = strings.TrimSpace(token)
	if token == "" {
		return SessionRecord{}, false
	}
	key := sessionStorageKey(token)
	session, ok := sessions[key]
	if ok {
		return normalizeSessionRecord(key, session), true
	}
	session, ok = sessions[token]
	if ok {
		return normalizeSessionRecord(token, session), true
	}
	return session, ok
}

func normalizeSessionRecord(storageKey string, session SessionRecord) SessionRecord {
	storageKey = strings.TrimSpace(storageKey)
	if strings.TrimSpace(session.ID) == "" {
		session.ID = sessionIDFromStorageKey(storageKey, session)
	}
	if strings.TrimSpace(session.CreatedAt) == "" {
		session.CreatedAt = nowISO()
	}
	if strings.TrimSpace(session.LastSeenAt) == "" {
		session.LastSeenAt = session.CreatedAt
	}
	if strings.TrimSpace(session.ExpiresAt) == "" {
		session.ExpiresAt = time.Now().Add(sessionMaxAge).Format(time.RFC3339)
	}
	session.IP = truncateString(strings.TrimSpace(session.IP), 80)
	session.UserAgent = truncateString(strings.TrimSpace(session.UserAgent), 360)
	session.Device = truncateString(firstNonEmpty(strings.TrimSpace(session.Device), deviceLabelFromUserAgent(session.UserAgent)), 120)
	return session
}

func sessionIDFromStorageKey(storageKey string, session SessionRecord) string {
	seed := strings.TrimSpace(storageKey)
	if seed == "" && strings.TrimSpace(session.Token) != "" {
		seed = sessionStorageKey(session.Token)
	}
	if seed == "" {
		seed = strings.Join([]string{session.UserID, session.CreatedAt, session.ExpiresAt}, "|")
	}
	hash := sha256.Sum256([]byte(seed))
	return "sess_" + hex.EncodeToString(hash[:])[:16]
}

func updateSessionRequestMetadata(session *SessionRecord, r *http.Request, at string) {
	if session == nil || r == nil {
		return
	}
	session.IP = clientIP(r)
	session.UserAgent = truncateString(strings.TrimSpace(r.UserAgent()), 360)
	session.Device = deviceLabelFromUserAgent(session.UserAgent)
	session.LastSeenAt = firstNonEmpty(at, nowISO())
}

func sessionStorageKey(token string) string {
	token = strings.TrimSpace(token)
	if token == "" {
		return "sha256:"
	}
	hash := sha256.Sum256([]byte(token))
	return "sha256:" + hex.EncodeToString(hash[:])
}

func hashAdminAPIToken(token string) string {
	token = strings.TrimSpace(token)
	if token == "" {
		return ""
	}
	hash := sha256.Sum256([]byte(token))
	return "sha256:" + hex.EncodeToString(hash[:])
}

func verifyAdminAPITokenHash(hashValue string, token string) bool {
	hashValue = strings.TrimSpace(hashValue)
	token = strings.TrimSpace(token)
	if hashValue == "" || token == "" {
		return false
	}
	if strings.HasPrefix(hashValue, "sha256:") {
		return constantTimeStringEqual(hashValue, hashAdminAPIToken(token))
	}
	return constantTimeStringEqual(hashValue, token)
}

func isSessionStorageKey(value string) bool {
	return strings.HasPrefix(value, "sha256:") && len(value) == len("sha256:")+sha256.Size*2
}

func setSessionCookieFromPayload(r *http.Request, payload any) {
	value, ok := payload.(response)
	if !ok {
		return
	}
	token, _ := value["token"].(string)
	if token == "" {
		return
	}
	setSessionCookie(responseWriterFromRequest(r), token)
}

func setSessionCookie(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    token,
		Path:     "/",
		MaxAge:   int(sessionMaxAge.Seconds()),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}

func clearSessionCookie(r *http.Request) {
	http.SetCookie(responseWriterFromRequest(r), &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}

func setOAuthStateCookie(w http.ResponseWriter, stateToken string) {
	http.SetCookie(w, &http.Cookie{
		Name:     oauthStateCookieName,
		Value:    stateToken,
		Path:     "/",
		MaxAge:   10 * 60,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}

func clearOAuthStateCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     oauthStateCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}

func responseWriterFromRequest(r *http.Request) http.ResponseWriter {
	if writer, ok := r.Context().Value(responseWriterKey{}).(http.ResponseWriter); ok {
		return writer
	}
	return nilResponseWriter{}
}

type responseWriterKey struct{}

type nilResponseWriter struct{}

func (nilResponseWriter) Header() http.Header       { return http.Header{} }
func (nilResponseWriter) Write([]byte) (int, error) { return 0, nil }
func (nilResponseWriter) WriteHeader(int)           {}

func sessionToken(r *http.Request) string {
	if cookie, err := r.Cookie(sessionCookieName); err == nil && strings.TrimSpace(cookie.Value) != "" {
		return strings.TrimSpace(cookie.Value)
	}
	header := strings.TrimSpace(r.Header.Get("Authorization"))
	if strings.HasPrefix(strings.ToLower(header), "bearer ") {
		return strings.TrimSpace(header[7:])
	}
	return ""
}

func adminTokenFromRequest(r *http.Request) string {
	if token := strings.TrimSpace(r.Header.Get("X-Admin-Token")); token != "" {
		return token
	}
	header := strings.TrimSpace(r.Header.Get("Authorization"))
	if strings.HasPrefix(strings.ToLower(header), "bearer ") {
		return strings.TrimSpace(header[7:])
	}
	return ""
}

func (app *App) storedAdminSecurity() AdminSecurityConfig {
	if app == nil || app.store == nil {
		return normalizeAdminSecurityConfig(AdminSecurityConfig{Username: "admin"})
	}
	state := app.store.snapshot()
	return normalizeAdminSecurityConfig(state.Config.Security)
}

func (app *App) effectiveAdminUsername(security AdminSecurityConfig) string {
	security = normalizeAdminSecurityConfig(security)
	if strings.TrimSpace(app.adminPassword) != "" || strings.TrimSpace(app.adminPasswordHash) != "" || strings.TrimSpace(app.adminAPIToken) != "" {
		return firstNonEmpty(strings.TrimSpace(app.adminUsername), security.Username, "admin")
	}
	return firstNonEmpty(security.Username, strings.TrimSpace(app.adminUsername), "admin")
}

func (app *App) verifyAdminPassword(password string) bool {
	security := app.storedAdminSecurity()
	return app.verifyAdminCredentials(app.effectiveAdminUsername(security), password, security)
}

func (app *App) verifyAdminCredentials(username string, password string, security AdminSecurityConfig) bool {
	username = strings.TrimSpace(username)
	password = strings.TrimSpace(password)
	if password == "" {
		return false
	}
	if app.adminPasswordHash != "" {
		return constantTimeStringEqual(username, app.adminUsername) && verifyPassword(app.adminPasswordHash, password)
	}
	if app.adminPassword != "" {
		return constantTimeStringEqual(username, app.adminUsername) && constantTimeStringEqual(password, app.adminPassword)
	}
	if app.adminAPIToken != "" {
		return constantTimeStringEqual(username, app.adminUsername) && constantTimeStringEqual(password, app.adminAPIToken)
	}
	security = normalizeAdminSecurityConfig(security)
	if security.PasswordHash != "" {
		return constantTimeStringEqual(username, security.Username) && verifyPassword(security.PasswordHash, password)
	}
	return false
}

func (app *App) createAdminSession(username string) AdminSessionRecord {
	now := time.Now()
	session := AdminSessionRecord{
		Token:     "adm_" + randomHex(32),
		Username:  username,
		CreatedAt: now.Format(time.RFC3339),
		ExpiresAt: now.Add(adminSessionMaxAge).Format(time.RFC3339),
	}
	app.adminSessionsMu.Lock()
	defer app.adminSessionsMu.Unlock()
	if app.adminSessions == nil {
		app.adminSessions = map[string]AdminSessionRecord{}
	}
	app.adminSessions[session.Token] = session
	return session
}

func (app *App) lookupAdminSession(token string) (AdminSessionRecord, bool) {
	token = strings.TrimSpace(token)
	if token == "" {
		return AdminSessionRecord{}, false
	}
	app.adminSessionsMu.Lock()
	defer app.adminSessionsMu.Unlock()
	session, ok := app.adminSessions[token]
	if !ok {
		return AdminSessionRecord{}, false
	}
	if adminSessionExpired(session) {
		delete(app.adminSessions, token)
		return AdminSessionRecord{}, false
	}
	return session, true
}

func (app *App) deleteAdminSession(token string) {
	token = strings.TrimSpace(token)
	if token == "" {
		return
	}
	app.adminSessionsMu.Lock()
	defer app.adminSessionsMu.Unlock()
	delete(app.adminSessions, token)
}

func adminSessionExpired(session AdminSessionRecord) bool {
	expiresAt, err := time.Parse(time.RFC3339, strings.TrimSpace(session.ExpiresAt))
	return err != nil || time.Now().After(expiresAt)
}

func constantTimeStringEqual(left string, right string) bool {
	leftBytes := []byte(left)
	rightBytes := []byte(right)
	if len(leftBytes) != len(rightBytes) {
		return false
	}
	return subtle.ConstantTimeCompare(leftBytes, rightBytes) == 1
}

func authenticatedUser(r *http.Request, state AppState) (UserRecord, bool) {
	token := sessionToken(r)
	session, ok := lookupSession(state.Sessions, token)
	if !ok || sessionExpired(session) {
		return UserRecord{}, false
	}
	user, ok := findUserByID(state.Users, session.UserID)
	if !ok || !accountActive(user.RegisteredUser) {
		return UserRecord{}, false
	}
	return user, true
}

func authenticatedUserIndex(token string, state *AppState) (int, bool) {
	session, ok := lookupSession(state.Sessions, token)
	if !ok || sessionExpired(session) {
		return -1, false
	}
	for index, user := range state.Users {
		if user.ID == session.UserID && accountActive(user.RegisteredUser) {
			return index, true
		}
	}
	return -1, false
}

func authenticatedSessionDetails(token string, state *AppState) (string, SessionRecord, UserRecord, bool) {
	if state == nil {
		return "", SessionRecord{}, UserRecord{}, false
	}
	token = strings.TrimSpace(token)
	if token == "" {
		return "", SessionRecord{}, UserRecord{}, false
	}
	currentKey := sessionStorageKey(token)
	session, ok := lookupSession(state.Sessions, token)
	if !ok || sessionExpired(session) {
		return "", SessionRecord{}, UserRecord{}, false
	}
	session = normalizeSessionRecord(currentKey, session)
	user, ok := findUserByID(state.Users, session.UserID)
	if !ok || !accountActive(user.RegisteredUser) {
		return "", SessionRecord{}, UserRecord{}, false
	}
	return currentKey, session, user, true
}

func securitySessionItems(sessions map[string]SessionRecord, userID string, currentKey string) []response {
	items := []response{}
	currentID := ""
	if current, ok := sessions[currentKey]; ok {
		currentID = normalizeSessionRecord(currentKey, current).ID
	}
	for key, session := range sessions {
		session = normalizeSessionRecord(key, session)
		if session.UserID != userID || sessionExpired(session) {
			continue
		}
		items = append(items, response{
			"id":         session.ID,
			"device":     session.Device,
			"ip":         session.IP,
			"userAgent":  session.UserAgent,
			"createdAt":  session.CreatedAt,
			"lastSeenAt": session.LastSeenAt,
			"expiresAt":  session.ExpiresAt,
			"current":    key == currentKey || (currentID != "" && session.ID == currentID),
		})
	}
	sort.SliceStable(items, func(i int, j int) bool {
		leftCurrent, _ := items[i]["current"].(bool)
		rightCurrent, _ := items[j]["current"].(bool)
		if leftCurrent != rightCurrent {
			return leftCurrent
		}
		left := stringFromAny(items[i]["lastSeenAt"])
		right := stringFromAny(items[j]["lastSeenAt"])
		if left != right {
			return left > right
		}
		leftCreated := stringFromAny(items[i]["createdAt"])
		rightCreated := stringFromAny(items[j]["createdAt"])
		if leftCreated != rightCreated {
			return leftCreated > rightCreated
		}
		return stringFromAny(items[i]["id"]) > stringFromAny(items[j]["id"])
	})
	return items
}

func findUserByID(users []UserRecord, id string) (UserRecord, bool) {
	for _, user := range users {
		if user.ID == id {
			return user, true
		}
	}
	return UserRecord{}, false
}

func findUserIndexByID(users []UserRecord, id string) (int, bool) {
	for index, user := range users {
		if user.ID == id {
			return index, true
		}
	}
	return -1, false
}

func normalizeRegisteredUser(user *RegisteredUser) {
	user.DisplayName = sanitizeCompanyMailText(user.DisplayName)
	user.Status = normalizeAccountStatus(user.Status)
	if user.Status != "disabled" {
		user.DisabledAt = ""
	}
	user.MailboxStatus = normalizeMailboxStatus(user.MailboxStatus)
	if user.MailboxStatus != "provisioned" {
		user.MailboxProvisionedAt = ""
	}
}

func sanitizeCompanyMailText(value string) string {
	replacer := strings.NewReplacer(
		legacyYuexiangFoodName(), "悦享账号",
		"\u4e1a\u52a1"+"\u9080\u8bf7", "通知邮件",
		"\u5546\u6237", "用户",
		"\u9a91\u624b", "用户",
	)
	return replacer.Replace(value)
}

func legacyYuexiangFoodName() string {
	return "\u60a6\u4eab" + "e" + "\u98df"
}

func normalizeAccountStatus(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "disabled":
		return "disabled"
	default:
		return "active"
	}
}

func accountActive(user RegisteredUser) bool {
	return normalizeAccountStatus(user.Status) != "disabled"
}

func normalizeMailboxStatus(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "provisioned", "queued", "failed", "pending_config":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return "pending_config"
	}
}

func emailExists(users []UserRecord, email string) bool {
	for _, user := range users {
		if strings.EqualFold(user.Email, email) {
			return true
		}
	}
	return false
}

func normalizeLoginMode(candidate string, identifier string, auth AuthConfig) string {
	candidate = strings.ToLower(strings.TrimSpace(candidate))
	if candidate == "phone" || candidate == "email" {
		return candidate
	}
	identifier = strings.TrimSpace(identifier)
	if strings.Contains(identifier, "@") {
		return "email"
	}
	if phoneRe.MatchString(normalizePhone(identifier)) {
		return "phone"
	}
	auth = normalizeAuthConfig(auth)
	if auth.PhoneLoginEnabled {
		return "phone"
	}
	if auth.EmailLoginEnabled {
		return "email"
	}
	return "phone"
}

func mailboxLocalPartFromInput(config MailboxConfig, value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", errors.New("请输入邮箱地址")
	}
	local := value
	if strings.Contains(value, "@") {
		address, err := netmail.ParseAddress(value)
		if err != nil {
			return "", errors.New("邮箱地址无效")
		}
		parts := strings.SplitN(strings.ToLower(strings.TrimSpace(address.Address)), "@", 2)
		if len(parts) != 2 || parts[0] == "" {
			return "", errors.New("邮箱地址无效")
		}
		if !strings.EqualFold(normalizeDomain(parts[1]), normalizeDomain(config.Domain)) {
			return "", fmt.Errorf("只能使用 @%s 的公司邮箱", normalizeDomain(config.Domain))
		}
		local = parts[0]
	}
	local = normalizeMailboxName(local)
	if local == "" {
		return "", errors.New("请输入邮箱地址")
	}
	selectedPrefix := ""
	if config.PrefixPolicyEnabled {
		for _, prefix := range normalizeAllowedPrefixes(config.AllowedPrefixes) {
			if strings.HasPrefix(local, prefix+"-") {
				selectedPrefix = prefix
				local = strings.TrimPrefix(local, prefix+"-")
				break
			}
		}
	}
	return buildMailboxLocalPart(config, local, selectedPrefix)
}

func mailboxEmailFromInput(config MailboxConfig, value string) (string, error) {
	localPart, err := mailboxLocalPartFromInput(config, value)
	if err != nil {
		return "", err
	}
	return localPart + "@" + normalizeDomain(config.Domain), nil
}

func activeInviteEmailExists(invites []InviteRecord, email string) bool {
	for _, invite := range invites {
		if inviteActive(invite) && strings.EqualFold(invite.Email, email) {
			return true
		}
	}
	return false
}

func inviteActive(invite InviteRecord) bool {
	if invite.UsedAt != "" {
		return false
	}
	if invite.ExpiresAt == "" {
		return true
	}
	expiresAt, err := time.Parse(time.RFC3339, invite.ExpiresAt)
	return err != nil || time.Now().Before(expiresAt)
}

func findInvite(invites []InviteRecord, code string) *InviteRecord {
	code = strings.ToUpper(strings.TrimSpace(code))
	if code == "" {
		return nil
	}
	for index := range invites {
		if invites[index].Code == code {
			return &invites[index]
		}
	}
	return nil
}

func markInviteUsed(invites []InviteRecord, code string, usedAt string) {
	for index := range invites {
		if invites[index].Code == code {
			invites[index].UsedAt = usedAt
			return
		}
	}
}

func updateRegisteredUser(users []RegisteredUser, next RegisteredUser) {
	normalizeRegisteredUser(&next)
	for index := range users {
		if users[index].ID == next.ID {
			users[index] = next
			return
		}
	}
}

func deleteSessionsForUser(sessions map[string]SessionRecord, userID string) {
	for key, session := range sessions {
		if session.UserID == userID {
			delete(sessions, key)
		}
	}
}

func appendAuditLog(state *AppState, r *http.Request, actor string, action string, target string, detail string) {
	log := AuditLogRecord{
		ID:        nextID("audit"),
		Actor:     firstNonEmpty(actor, "system"),
		Action:    strings.TrimSpace(action),
		Target:    strings.TrimSpace(target),
		Detail:    strings.TrimSpace(detail),
		IP:        clientIP(r),
		CreatedAt: nowISO(),
	}
	state.Config.AuditLogs = append([]AuditLogRecord{log}, state.Config.AuditLogs...)
	if len(state.Config.AuditLogs) > 500 {
		state.Config.AuditLogs = state.Config.AuditLogs[:500]
	}
}

func clientIP(r *http.Request) string {
	if r == nil {
		return ""
	}
	for _, header := range []string{"X-Forwarded-For", "X-Real-IP", "CF-Connecting-IP"} {
		value := strings.TrimSpace(r.Header.Get(header))
		if value == "" {
			continue
		}
		candidate := strings.TrimSpace(strings.Split(value, ",")[0])
		if candidate != "" {
			return truncateString(candidate, 80)
		}
	}
	host, _, err := net.SplitHostPort(strings.TrimSpace(r.RemoteAddr))
	if err == nil && host != "" {
		return truncateString(host, 80)
	}
	return truncateString(r.RemoteAddr, 80)
}

func normalizeSendMessagePayload(payload SendMessagePayload) (SendMessagePayload, error) {
	recipients, err := normalizeEmailList(payload.Recipients, true)
	if err != nil {
		return payload, err
	}
	cc, err := normalizeEmailList(payload.CC, false)
	if err != nil {
		return payload, err
	}
	bcc, err := normalizeEmailList(payload.BCC, false)
	if err != nil {
		return payload, err
	}
	payload.Recipients = recipients
	payload.CC = cc
	payload.BCC = bcc
	payload.Subject = truncateRunes(firstNonEmpty(strings.TrimSpace(payload.Subject), "未命名邮件"), 240)
	payload.Body = normalizeMessageBody(payload.Body)
	attachments, err := normalizeAttachmentList(payload.Attachments)
	if err != nil {
		return payload, err
	}
	payload.Attachments = attachments
	payload.TemplateID = strings.TrimSpace(payload.TemplateID)
	payload.ReplyToMessageID = strings.TrimSpace(payload.ReplyToMessageID)
	payload.Source = normalizeSendSource(payload.Source)
	return payload, nil
}

func normalizeSaveDraftPayload(payload SaveDraftPayload) (SaveDraftPayload, error) {
	recipients, err := normalizeEmailList(payload.Recipients, false)
	if err != nil {
		return payload, err
	}
	cc, err := normalizeEmailList(payload.CC, false)
	if err != nil {
		return payload, err
	}
	bcc, err := normalizeEmailList(payload.BCC, false)
	if err != nil {
		return payload, err
	}
	payload.DraftID = strings.TrimSpace(payload.DraftID)
	payload.Recipients = recipients
	payload.CC = cc
	payload.BCC = bcc
	payload.Subject = truncateRunes(strings.TrimSpace(payload.Subject), 240)
	payload.Body = normalizeMessageBody(payload.Body)
	attachments, err := normalizeAttachmentList(payload.Attachments)
	if err != nil {
		return payload, err
	}
	payload.Attachments = attachments
	return payload, nil
}

func normalizeEmailList(values []string, required bool) ([]string, error) {
	out := make([]string, 0, len(values))
	seen := map[string]bool{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		address, err := netmail.ParseAddress(value)
		if err != nil || strings.TrimSpace(address.Address) == "" {
			return nil, fmt.Errorf("邮箱地址无效: %s", value)
		}
		normalized := strings.ToLower(strings.TrimSpace(address.Address))
		if seen[normalized] {
			continue
		}
		seen[normalized] = true
		out = append(out, normalized)
	}
	if len(out) > 100 {
		return nil, errors.New("收件人不能超过 100 个")
	}
	if required && len(out) == 0 {
		return nil, errors.New("请填写收件人")
	}
	return out, nil
}

func normalizeMessageBody(body MessageBodyPayload) MessageBodyPayload {
	body.Format = strings.ToLower(strings.TrimSpace(body.Format))
	switch body.Format {
	case "html", "both":
	default:
		body.Format = "text"
	}
	body.Text = strings.TrimSpace(body.Text)
	body.HTML = strings.TrimSpace(body.HTML)
	if body.Format == "html" && body.HTML == "" {
		body.Format = "text"
	}
	if body.Format == "both" && body.HTML == "" {
		body.Format = "text"
	}
	if body.Format == "text" && body.Text == "" && body.HTML != "" {
		body.Format = "html"
	}
	return body
}

func normalizeAttachmentList(attachments []any) ([]any, error) {
	if len(attachments) == 0 {
		return []any{}, nil
	}
	if len(attachments) > maxAttachmentCount {
		return nil, fmt.Errorf("附件不能超过 %d 个", maxAttachmentCount)
	}
	out := make([]any, 0, len(attachments))
	var totalBytes int64
	for index, attachment := range attachments {
		normalized, sizeBytes, err := normalizeAttachmentPayload(attachment, index)
		if err != nil {
			return nil, err
		}
		totalBytes += sizeBytes
		if totalBytes > maxMessageAttachmentBytes {
			return nil, errors.New("附件总大小不能超过 25 MB")
		}
		out = append(out, normalized)
	}
	return out, nil
}

func normalizeAttachmentPayload(attachment any, index int) (map[string]any, int64, error) {
	source, ok := attachment.(map[string]any)
	if !ok {
		raw, err := json.Marshal(attachment)
		if err != nil {
			return nil, 0, errors.New("附件格式无效")
		}
		if err := json.Unmarshal(raw, &source); err != nil {
			return nil, 0, errors.New("附件格式无效")
		}
	}
	name := truncateRunes(firstNonEmpty(
		stringFromAny(source["name"]),
		stringFromAny(source["filename"]),
		fmt.Sprintf("attachment-%d", index+1),
	), 255)
	contentType := truncateRunes(firstNonEmpty(stringFromAny(source["contentType"]), stringFromAny(source["content_type"]), "application/octet-stream"), 128)
	attachmentType := truncateRunes(firstNonEmpty(stringFromAny(source["type"]), contentType, "file"), 64)
	sizeBytes, ok := numericAnyToInt64(source["sizeBytes"])
	if !ok || sizeBytes < 0 {
		sizeBytes, _ = numericAnyToInt64(source["size"])
	}
	contentBase64 := strings.TrimSpace(firstNonEmpty(stringFromAny(source["contentBase64"]), stringFromAny(source["content_base64"])))
	if contentBase64 != "" {
		decoded, err := base64.StdEncoding.DecodeString(contentBase64)
		if err != nil {
			return nil, 0, fmt.Errorf("附件 %s 内容不是有效 base64", name)
		}
		if sizeBytes <= 0 {
			sizeBytes = int64(len(decoded))
		}
	}
	if sizeBytes > maxAttachmentBytes {
		return nil, 0, fmt.Errorf("单个附件不能超过 5 MB: %s", name)
	}
	normalized := map[string]any{
		"id":          firstNonEmpty(stringFromAny(source["id"]), stringFromAny(source["assetId"]), nextID("att")),
		"assetId":     nullableString(firstNonEmpty(stringFromAny(source["assetId"]), stringFromAny(source["asset_id"]))),
		"name":        name,
		"type":        attachmentType,
		"contentType": nullableString(contentType),
		"sizeBytes":   sizeBytes,
		"sizeLabel":   firstNonEmpty(stringFromAny(source["sizeLabel"]), stringFromAny(source["size"]), formatBytes(sizeBytes)),
		"downloadUrl": nullableString(firstNonEmpty(stringFromAny(source["downloadUrl"]), stringFromAny(source["url"]))),
		"previewUrl":  nullableString(stringFromAny(source["previewUrl"])),
	}
	if contentBase64 != "" {
		normalized["contentEncoding"] = "base64"
		normalized["contentBase64"] = contentBase64
	}
	return normalized, sizeBytes, nil
}

func (app *App) persistAttachmentList(accountID string, attachments []any, apiBasePath string) ([]any, error) {
	if len(attachments) == 0 {
		return []any{}, nil
	}
	out := make([]any, 0, len(attachments))
	for _, attachment := range attachments {
		source, err := attachmentMap(attachment)
		if err != nil {
			return nil, err
		}
		contentBase64 := strings.TrimSpace(firstNonEmpty(stringFromAny(source["contentBase64"]), stringFromAny(source["content_base64"])))
		if contentBase64 == "" {
			out = append(out, app.attachmentPublicMetadata(accountID, source, apiBasePath))
			continue
		}
		decoded, err := base64.StdEncoding.DecodeString(contentBase64)
		if err != nil {
			return nil, fmt.Errorf("附件 %s 内容不是有效 base64", firstNonEmpty(stringFromAny(source["name"]), "未命名附件"))
		}
		if len(decoded) > maxAttachmentBytes {
			return nil, fmt.Errorf("单个附件不能超过 5 MB: %s", firstNonEmpty(stringFromAny(source["name"]), "未命名附件"))
		}
		assetID := firstNonEmpty(stringFromAny(source["assetId"]), stringFromAny(source["asset_id"]))
		if !validAssetID(assetID) {
			assetID = nextID("asset")
		}
		filePath, metaPath, err := app.attachmentAssetPaths(accountID, assetID)
		if err != nil {
			return nil, err
		}
		if err := os.MkdirAll(filepath.Dir(filePath), 0o700); err != nil {
			return nil, fmt.Errorf("创建附件目录失败: %w", err)
		}
		if err := os.WriteFile(filePath, decoded, 0o600); err != nil {
			return nil, fmt.Errorf("保存附件失败: %w", err)
		}
		sum := sha256.Sum256(decoded)
		record := AttachmentAssetRecord{
			ID:          assetID,
			AccountID:   accountID,
			Name:        firstNonEmpty(stringFromAny(source["name"]), stringFromAny(source["filename"]), "未命名附件"),
			Type:        firstNonEmpty(stringFromAny(source["type"]), "file"),
			ContentType: firstNonEmpty(stringFromAny(source["contentType"]), stringFromAny(source["content_type"]), "application/octet-stream"),
			SizeBytes:   int64(len(decoded)),
			SHA256:      hex.EncodeToString(sum[:]),
			CreatedAt:   nowISO(),
		}
		raw, err := json.MarshalIndent(record, "", "  ")
		if err != nil {
			return nil, fmt.Errorf("编码附件元数据失败: %w", err)
		}
		if err := os.WriteFile(metaPath, raw, 0o600); err != nil {
			return nil, fmt.Errorf("保存附件元数据失败: %w", err)
		}
		source["assetId"] = assetID
		source["sizeBytes"] = record.SizeBytes
		source["sizeLabel"] = formatBytes(record.SizeBytes)
		source["contentType"] = record.ContentType
		out = append(out, app.attachmentPublicMetadata(accountID, source, apiBasePath))
	}
	return out, nil
}

func (app *App) hydrateAttachmentList(accountID string, attachments []any) ([]any, error) {
	if len(attachments) == 0 {
		return []any{}, nil
	}
	out := make([]any, 0, len(attachments))
	for _, attachment := range attachments {
		source, err := attachmentMap(attachment)
		if err != nil {
			return nil, err
		}
		if strings.TrimSpace(firstNonEmpty(stringFromAny(source["contentBase64"]), stringFromAny(source["content_base64"]))) != "" {
			out = append(out, source)
			continue
		}
		assetID := firstNonEmpty(stringFromAny(source["assetId"]), stringFromAny(source["asset_id"]))
		if assetID == "" {
			out = append(out, source)
			continue
		}
		record, filePath, err := app.loadAttachmentAsset(accountID, assetID)
		if err != nil {
			return nil, fmt.Errorf("附件 %s 不存在，请重新上传", firstNonEmpty(stringFromAny(source["name"]), assetID))
		}
		raw, err := os.ReadFile(filePath)
		if err != nil {
			return nil, fmt.Errorf("读取附件 %s 失败: %w", firstNonEmpty(record.Name, assetID), err)
		}
		if len(raw) > maxAttachmentBytes {
			return nil, fmt.Errorf("单个附件不能超过 5 MB: %s", firstNonEmpty(record.Name, assetID))
		}
		source["assetId"] = record.ID
		source["name"] = firstNonEmpty(stringFromAny(source["name"]), record.Name)
		source["type"] = firstNonEmpty(stringFromAny(source["type"]), record.Type)
		source["contentType"] = firstNonEmpty(stringFromAny(source["contentType"]), record.ContentType)
		source["sizeBytes"] = record.SizeBytes
		source["sizeLabel"] = firstNonEmpty(stringFromAny(source["sizeLabel"]), formatBytes(record.SizeBytes))
		source["contentEncoding"] = "base64"
		source["contentBase64"] = base64.StdEncoding.EncodeToString(raw)
		out = append(out, source)
	}
	return out, nil
}

func (app *App) attachmentPublicMetadata(accountID string, source map[string]any, apiBasePath string) map[string]any {
	metadata := map[string]any{}
	for key, value := range source {
		switch key {
		case "contentBase64", "content_base64", "contentEncoding", "content_encoding":
			continue
		default:
			metadata[key] = value
		}
	}
	assetID := firstNonEmpty(stringFromAny(metadata["assetId"]), stringFromAny(metadata["asset_id"]))
	if validAssetID(assetID) {
		metadata["assetId"] = assetID
		if strings.TrimSpace(stringFromAny(metadata["downloadUrl"])) == "" {
			metadata["downloadUrl"] = attachmentDownloadURL(apiBasePath, assetID)
		}
	}
	if strings.TrimSpace(stringFromAny(metadata["id"])) == "" {
		metadata["id"] = firstNonEmpty(assetID, nextID("att"))
	}
	if strings.TrimSpace(stringFromAny(metadata["name"])) == "" {
		metadata["name"] = "未命名附件"
	}
	if strings.TrimSpace(stringFromAny(metadata["type"])) == "" {
		metadata["type"] = "file"
	}
	if _, ok := numericAnyToInt64(metadata["sizeBytes"]); !ok {
		metadata["sizeBytes"] = int64(0)
	}
	if strings.TrimSpace(stringFromAny(metadata["sizeLabel"])) == "" {
		if sizeBytes, ok := numericAnyToInt64(metadata["sizeBytes"]); ok {
			metadata["sizeLabel"] = formatBytes(sizeBytes)
		} else {
			metadata["sizeLabel"] = "0 B"
		}
	}
	_ = accountID
	return metadata
}

func attachmentMap(attachment any) (map[string]any, error) {
	source, ok := attachment.(map[string]any)
	if ok {
		out := make(map[string]any, len(source))
		for key, value := range source {
			out[key] = value
		}
		return out, nil
	}
	raw, err := json.Marshal(attachment)
	if err != nil {
		return nil, errors.New("附件格式无效")
	}
	if err := json.Unmarshal(raw, &source); err != nil {
		return nil, errors.New("附件格式无效")
	}
	if source == nil {
		return nil, errors.New("附件格式无效")
	}
	return source, nil
}

func (app *App) loadAttachmentAsset(accountID string, assetID string) (AttachmentAssetRecord, string, error) {
	if !validAssetID(assetID) {
		return AttachmentAssetRecord{}, "", errors.New("附件不存在")
	}
	filePath, metaPath, err := app.attachmentAssetPaths(accountID, assetID)
	if err != nil {
		return AttachmentAssetRecord{}, "", err
	}
	raw, err := os.ReadFile(metaPath)
	if err != nil {
		return AttachmentAssetRecord{}, "", err
	}
	var record AttachmentAssetRecord
	if err := json.Unmarshal(raw, &record); err != nil {
		return AttachmentAssetRecord{}, "", err
	}
	if record.AccountID != accountID || record.ID != assetID {
		return AttachmentAssetRecord{}, "", errors.New("附件不存在")
	}
	if stat, err := os.Stat(filePath); err != nil {
		return AttachmentAssetRecord{}, "", err
	} else if record.SizeBytes <= 0 {
		record.SizeBytes = stat.Size()
	}
	return record, filePath, nil
}

func (app *App) attachmentAssetPaths(accountID string, assetID string) (string, string, error) {
	if !validAssetID(assetID) {
		return "", "", errors.New("附件编号无效")
	}
	accountDir := filepath.Join(app.resolvedAttachmentDir(), safePathSegment(accountID))
	filePath := filepath.Join(accountDir, assetID+".bin")
	metaPath := filepath.Join(accountDir, assetID+".json")
	return filePath, metaPath, nil
}

func (app *App) resolvedAttachmentDir() string {
	dir := strings.TrimSpace(app.attachmentDir)
	if dir == "" {
		dir = env("ATTACHMENT_DIR", filepath.Join(".data", "attachments"))
	}
	return filepath.Clean(dir)
}

func validAssetID(assetID string) bool {
	return assetIDRe.MatchString(strings.TrimSpace(assetID))
}

func safePathSegment(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "unknown"
	}
	var builder strings.Builder
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' || r == '.' {
			builder.WriteRune(r)
		} else {
			builder.WriteByte('_')
		}
	}
	out := strings.Trim(builder.String(), ".")
	if out == "" {
		return "unknown"
	}
	return out
}

func attachmentDownloadURL(apiBasePath string, assetID string) string {
	base := strings.TrimRight(firstNonEmpty(apiBasePath, "/api/v1/post-office"), "/")
	return base + "/attachments/" + url.PathEscape(assetID) + "/download"
}

func contentDispositionAttachment(name string) string {
	name = strings.ReplaceAll(firstNonEmpty(name, "attachment"), "\n", " ")
	name = strings.ReplaceAll(name, "\r", " ")
	return `attachment; filename="` + strings.ReplaceAll(name, `"`, `'`) + `"; filename*=UTF-8''` + url.PathEscape(name)
}

func formatBytes(sizeBytes int64) string {
	if sizeBytes >= 1024*1024 {
		return fmt.Sprintf("%.1f MB", float64(sizeBytes)/(1024*1024))
	}
	if sizeBytes >= 1024 {
		return fmt.Sprintf("%d KB", (sizeBytes+1023)/1024)
	}
	return fmt.Sprintf("%d B", sizeBytes)
}

func normalizeSendSource(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "invite", "workflow", "system":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return "manual"
	}
}

func buildOutgoingMessage(user UserRecord, payload SendMessagePayload, now time.Time) MailMessage {
	bodyText := firstNonEmpty(payload.Body.Text, stripHTML(payload.Body.HTML), payload.Subject)
	sortAt := now.Format(time.RFC3339)
	return MailMessage{
		ID:             nextID("mail"),
		ThreadID:       firstNonEmpty(payload.ReplyToMessageID, ""),
		Folder:         "sent",
		PreviousFolder: "sent",
		Sender:         firstNonEmpty(user.DisplayName, "InfiniteMail 用户"),
		SenderEmail:    user.Email,
		Recipients:     payload.Recipients,
		Avatar:         avatarInitial(firstNonEmpty(user.DisplayName, user.Email, "I")),
		Role:           "悦享用户",
		Subject:        payload.Subject,
		Snippet:        truncateRunes(bodyText, 120),
		Time:           "刚刚",
		DateTimeLabel:  now.Format("2006年1月2日 15:04"),
		SortAt:         sortAt,
		SentAt:         sortAt,
		IsUnread:       false,
		IsStarred:      false,
		HasAttachment:  len(payload.Attachments) > 0,
		Tags:           []string{"已发送"},
		IsOutgoing:     true,
		Content:        composeBodyHTML(payload.Body, payload.Subject),
		Attachments:    payload.Attachments,
		Source:         "mailbox",
		DeliveryStatus: "accepted",
		AcceptedAt:     sortAt,
	}
}

func buildDraftMessage(user UserRecord, payload SaveDraftPayload, now time.Time) MailMessage {
	subject := firstNonEmpty(payload.Subject, "未命名草稿")
	bodyText := firstNonEmpty(payload.Body.Text, stripHTML(payload.Body.HTML), "草稿已保存")
	sortAt := now.Format(time.RFC3339)
	return MailMessage{
		ID:             firstNonEmpty(payload.DraftID, nextID("draft")),
		Folder:         "drafts",
		PreviousFolder: "drafts",
		Sender:         firstNonEmpty(user.DisplayName, "InfiniteMail 用户"),
		SenderEmail:    user.Email,
		Recipients:     payload.Recipients,
		Avatar:         avatarInitial(firstNonEmpty(user.DisplayName, user.Email, "I")),
		Role:           "悦享用户",
		Subject:        subject,
		Snippet:        truncateRunes(bodyText, 120),
		Time:           "刚刚",
		DateTimeLabel:  now.Format("2006年1月2日 15:04"),
		SortAt:         sortAt,
		IsUnread:       false,
		IsStarred:      false,
		HasAttachment:  len(payload.Attachments) > 0,
		Tags:           []string{"草稿"},
		IsOutgoing:     true,
		Content:        composeBodyHTML(payload.Body, subject),
		Attachments:    payload.Attachments,
		Source:         "draft",
		DeliveryStatus: "draft",
	}
}

func mergeRelayMessage(fallback MailMessage, remote MailMessage) MailMessage {
	if strings.TrimSpace(remote.ID) == "" {
		return fallback
	}
	remote.Folder = firstNonEmpty(remote.Folder, fallback.Folder)
	remote.PreviousFolder = firstNonEmpty(remote.PreviousFolder, remote.Folder)
	remote.Sender = firstNonEmpty(remote.Sender, fallback.Sender)
	remote.SenderEmail = firstNonEmpty(remote.SenderEmail, fallback.SenderEmail)
	if len(remote.Recipients) == 0 {
		remote.Recipients = fallback.Recipients
	}
	remote.Avatar = firstNonEmpty(remote.Avatar, fallback.Avatar)
	remote.Role = firstNonEmpty(remote.Role, fallback.Role)
	remote.Subject = firstNonEmpty(remote.Subject, fallback.Subject)
	remote.Snippet = firstNonEmpty(remote.Snippet, fallback.Snippet)
	remote.Time = firstNonEmpty(remote.Time, fallback.Time)
	remote.DateTimeLabel = firstNonEmpty(remote.DateTimeLabel, fallback.DateTimeLabel)
	remote.SortAt = firstNonEmpty(remote.SortAt, fallback.SortAt)
	remote.SentAt = firstNonEmpty(remote.SentAt, fallback.SentAt)
	remote.ReceivedAt = firstNonEmpty(remote.ReceivedAt, fallback.ReceivedAt)
	remote.Tags = nonNilStrings(firstNonEmptySlice(remote.Tags, fallback.Tags))
	remote.IsOutgoing = remote.IsOutgoing || fallback.IsOutgoing
	remote.Content = firstNonEmpty(remote.Content, fallback.Content)
	remote.Attachments = nonNilAnys(firstNonEmptyAnySlice(remote.Attachments, fallback.Attachments))
	remote.Source = firstNonEmpty(remote.Source, fallback.Source)
	remote.DeliveryStatus = firstNonEmpty(remote.DeliveryStatus, fallback.DeliveryStatus)
	remote.AcceptedAt = firstNonEmpty(remote.AcceptedAt, fallback.AcceptedAt)
	remote.ProviderMessageID = firstNonEmpty(remote.ProviderMessageID, fallback.ProviderMessageID)
	remote.DeliveryError = firstNonEmpty(remote.DeliveryError, fallback.DeliveryError)
	return remote
}

func normalizeRemoteMailMessage(message MailMessage, user UserRecord, fallbackFolder string, now time.Time) MailMessage {
	folder := normalizeMessageFolder(message.Folder, normalizeMessageFolder(fallbackFolder, "inbox"))
	message.Folder = folder
	message.PreviousFolder = normalizeMessageFolder(message.PreviousFolder, folder)
	message.SenderEmail = firstNonEmpty(message.SenderEmail, "unknown@"+fallbackDomain(user.Email))
	message.Sender = sanitizeCompanyMailText(firstNonEmpty(message.Sender, message.SenderEmail))
	if len(message.Recipients) == 0 {
		message.Recipients = []string{user.Email}
	}
	message.Subject = truncateRunes(sanitizeCompanyMailText(firstNonEmpty(message.Subject, "未命名邮件")), 240)
	message.Content = sanitizeCompanyMailText(firstNonEmpty(message.Content, composeBodyHTML(MessageBodyPayload{Text: firstNonEmpty(message.Snippet, message.Subject)}, message.Subject)))
	message.Snippet = truncateRunes(sanitizeCompanyMailText(firstNonEmpty(message.Snippet, stripHTML(message.Content), message.Subject)), 120)
	message.SortAt = firstNonEmpty(message.SortAt, message.SentAt, message.ReceivedAt, now.Format(time.RFC3339))
	if strings.TrimSpace(message.ID) == "" {
		message.ID = stableRemoteMessageID(message)
	}
	message.Time = firstNonEmpty(message.Time, formatMailTimeLabel(message.SortAt))
	message.DateTimeLabel = firstNonEmpty(message.DateTimeLabel, formatMailDateTimeLabel(message.SortAt))
	message.Avatar = firstNonEmpty(message.Avatar, avatarInitial(firstNonEmpty(message.Sender, message.SenderEmail, "I")))
	message.Role = sanitizeCompanyMailText(firstNonEmpty(message.Role, "邮件联系人"))
	message.Tags = nonNilStrings(message.Tags)
	message.Attachments = nonNilAnys(message.Attachments)
	message.HasAttachment = message.HasAttachment || len(message.Attachments) > 0
	message.ProviderMessageID = strings.TrimSpace(message.ProviderMessageID)
	message.AcceptedAt = firstNonEmpty(message.AcceptedAt, message.SentAt)
	message.DeliveryError = strings.TrimSpace(message.DeliveryError)
	if folder == "sent" || folder == "drafts" {
		message.IsOutgoing = true
	}
	if folder == "sent" {
		message.SentAt = firstNonEmpty(message.SentAt, message.SortAt)
	}
	if folder == "inbox" {
		message.ReceivedAt = firstNonEmpty(message.ReceivedAt, message.SortAt)
	}
	message.Source = firstNonEmpty(message.Source, "mailbox")
	if folder == "drafts" {
		message.DeliveryStatus = firstNonEmpty(message.DeliveryStatus, "draft")
	} else if message.IsOutgoing {
		message.DeliveryStatus = firstNonEmpty(message.DeliveryStatus, "accepted")
	} else {
		message.DeliveryStatus = firstNonEmpty(message.DeliveryStatus, "received")
	}
	return message
}

func stableRemoteMessageID(message MailMessage) string {
	source := strings.Join([]string{
		message.Folder,
		message.SenderEmail,
		strings.Join(message.Recipients, ","),
		message.Subject,
		message.SortAt,
	}, "|")
	hash := sha256.Sum256([]byte(source))
	return "remote-" + hex.EncodeToString(hash[:])[:16]
}

func fallbackDomain(email string) string {
	_, domain := splitEmailAddress(email)
	if domain == "" {
		return "local"
	}
	return domain
}

func formatMailTimeLabel(value string) string {
	parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(value))
	if err != nil {
		return "刚刚"
	}
	if time.Since(parsed) < 2*time.Minute && time.Since(parsed) >= 0 {
		return "刚刚"
	}
	return parsed.Format("15:04")
}

func formatMailDateTimeLabel(value string) string {
	parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(value))
	if err != nil {
		return value
	}
	return parsed.Format("2006年1月2日 15:04")
}

func composeBodyHTML(body MessageBodyPayload, fallback string) string {
	if strings.TrimSpace(body.HTML) != "" {
		return body.HTML
	}
	text := firstNonEmpty(body.Text, fallback)
	escaped := strings.ReplaceAll(htmlEscape(text), "\n", "<br />")
	return `<p class="text-slate-700 leading-relaxed whitespace-pre-wrap">` + escaped + `</p>`
}

func stripHTML(value string) string {
	replacer := strings.NewReplacer("<br>", "\n", "<br/>", "\n", "<br />", "\n", "</p>", "\n", "</div>", "\n")
	value = replacer.Replace(value)
	value = regexp.MustCompile(`<[^>]+>`).ReplaceAllString(value, " ")
	return strings.Join(strings.Fields(value), " ")
}

func htmlEscape(value string) string {
	replacer := strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		`"`, "&quot;",
		"'", "&#39;",
	)
	return replacer.Replace(value)
}

func avatarInitial(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "I"
	}
	runes := []rune(value)
	if len(runes) == 0 {
		return "I"
	}
	return strings.ToUpper(string(runes[0]))
}

func truncateRunes(value string, limit int) string {
	value = strings.TrimSpace(value)
	if limit <= 0 {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit])
}

func upsertUserMessage(messages []MailMessage, next MailMessage) []MailMessage {
	for index := range messages {
		if messages[index].ID == next.ID {
			messages[index] = next
			return messages
		}
	}
	return append([]MailMessage{next}, messages...)
}

func findMessageByID(messages []MailMessage, id string) (MailMessage, bool) {
	index := findMessageIndexByID(messages, id)
	if index < 0 {
		return MailMessage{}, false
	}
	return messages[index], true
}

func findMessageIndexByID(messages []MailMessage, id string) int {
	id = strings.TrimSpace(id)
	if id == "" {
		return -1
	}
	for index, message := range messages {
		if message.ID == id {
			return index
		}
	}
	return -1
}

func normalizeMessageFolder(value string, fallback string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "inbox", "sent", "drafts", "trash", "archive", "starred":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return fallback
	}
}

func firstNonEmptySlice(values ...[]string) []string {
	for _, value := range values {
		if len(value) > 0 {
			return value
		}
	}
	return nil
}

func firstNonEmptyAnySlice(values ...[]any) []any {
	for _, value := range values {
		if len(value) > 0 {
			return value
		}
	}
	return nil
}

func nullableString(value string) any {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return value
}

func hasAutoReplyForMessage(messages []MailMessage, inboundMessageID string) bool {
	inboundMessageID = strings.TrimSpace(inboundMessageID)
	if inboundMessageID == "" {
		return false
	}
	for _, message := range messages {
		if message.Source == "auto_reply" && message.ThreadID == inboundMessageID {
			return true
		}
	}
	return false
}

func shouldSendAutoReply(user UserRecord, settings MailSettings, message MailMessage, payload InboundMailPayload) bool {
	if !settings.AutoReplyEnabled {
		return false
	}
	senderEmail := strings.ToLower(strings.TrimSpace(message.SenderEmail))
	if senderEmail == "" || strings.EqualFold(senderEmail, user.Email) {
		return false
	}
	subject := strings.ToLower(strings.TrimSpace(message.Subject))
	if strings.Contains(subject, "自动回复") || strings.Contains(subject, "auto-reply") || strings.Contains(subject, "autoreply") {
		return false
	}
	headers := normalizedMailHeaders(payload.Headers)
	autoSubmitted := strings.ToLower(headers["auto-submitted"])
	if autoSubmitted != "" && autoSubmitted != "no" {
		return false
	}
	precedence := strings.ToLower(headers["precedence"])
	if precedence == "bulk" || precedence == "list" || precedence == "junk" {
		return false
	}
	if headers["list-id"] != "" || headers["list-unsubscribe"] != "" || headers["x-auto-response-suppress"] != "" {
		return false
	}
	return true
}

func normalizedMailHeaders(headers map[string]string) map[string]string {
	out := map[string]string{}
	for key, value := range headers {
		key = strings.ToLower(strings.TrimSpace(key))
		if key == "" {
			continue
		}
		out[key] = strings.TrimSpace(value)
	}
	return out
}

func mailServerCanSend(config MailServerConfig) bool {
	config = normalizeMailServerConfig(config)
	return mailServerMessageSendEndpoint(config) != "" || smtpOutboundReady(config)
}

func autoReplySubject(subject string) string {
	subject = strings.TrimSpace(subject)
	if subject == "" {
		return "自动回复"
	}
	if strings.HasPrefix(strings.ToLower(subject), "re:") {
		return subject
	}
	return "Re: " + subject
}

func autoReplyBody(settings MailSettings, inbound MailMessage) MessageBodyPayload {
	message := firstNonEmpty(strings.TrimSpace(settings.AutoReplyMessage), "您好，我暂时无法及时回复，稍后会第一时间处理您的来信。")
	text := message
	if strings.TrimSpace(inbound.Subject) != "" {
		text += "\n\n原邮件主题：" + inbound.Subject
	}
	return MessageBodyPayload{Format: "text", Text: text}
}

func (app *App) sendAutoReply(ctx context.Context, config AdminConfig, user UserRecord, settings MailSettings, inbound MailMessage, r *http.Request) (MailMessage, error) {
	server := normalizeMailServerConfig(config.Mailbox.Server)
	if !mailServerCanSend(server) {
		return MailMessage{}, errors.New("自动回复发信通道未配置")
	}
	payload, err := normalizeSendMessagePayload(SendMessagePayload{
		Recipients:       []string{inbound.SenderEmail},
		Subject:          autoReplySubject(inbound.Subject),
		Body:             autoReplyBody(settings, inbound),
		Attachments:      []any{},
		ReplyToMessageID: inbound.ID,
		Source:           "system",
	})
	if err != nil {
		return MailMessage{}, err
	}
	now := time.Now()
	message := buildOutgoingMessage(user, payload, now)
	message.ThreadID = inbound.ID
	message.Source = "auto_reply"
	message.Tags = []string{"自动回复"}
	message.Role = "系统自动回复"
	remoteResult, err := relayMailboxMessage(ctx, config, user, message, payload, SaveDraftPayload{}, "send")
	if err != nil {
		return MailMessage{}, err
	}
	message = mergeRelayMessage(message, remoteResult.Message)
	message.AcceptedAt = firstNonEmpty(remoteResult.AcceptedAt, message.SentAt, now.Format(time.RFC3339))
	message.ProviderMessageID = remoteResult.ProviderMessageID
	message.DeliveryError = ""
	message.DeliveryStatus = firstNonEmpty(message.DeliveryStatus, "accepted")
	_, err = app.store.mutate(func(state *AppState) (any, error) {
		state.Messages[user.ID] = upsertUserMessage(state.Messages[user.ID], message)
		appendAuditLog(state, r, "system", "mail.auto_reply.sent", inbound.ID, inbound.SenderEmail)
		return nil, nil
	})
	if err != nil {
		return MailMessage{}, err
	}
	return message, nil
}

func (app *App) contactSourceMessages(ctx context.Context, user UserRecord, state AppState) ([]MailMessage, error) {
	messages := append([]MailMessage{}, state.Messages[user.ID]...)
	server := normalizeMailServerConfig(state.Config.Mailbox.Server)
	if server.Enabled || mailServerStrictDataPlane(server) {
		remoteCalled := false
		for _, folderID := range []string{"inbox", "sent", "drafts", "trash", "archive"} {
			remoteList, err := fetchMailboxMessageList(ctx, state.Config, user, folderID, "", "")
			if err != nil {
				return nil, err
			}
			if !remoteList.Called {
				continue
			}
			remoteCalled = true
			for _, message := range remoteList.Items {
				messages = upsertUserMessage(messages, message)
			}
		}
		if remoteCalled {
			if _, err := app.store.mutate(func(state *AppState) (any, error) {
				for _, message := range messages {
					state.Messages[user.ID] = upsertUserMessage(state.Messages[user.ID], message)
				}
				return nil, nil
			}); err != nil {
				return nil, err
			}
		}
	}
	return messages, nil
}

func buildContactsFromMessages(user UserRecord, messages []MailMessage, search string) []ContactRecord {
	selfEmail := strings.ToLower(strings.TrimSpace(user.Email))
	contactsByEmail := map[string]ContactRecord{}
	for _, message := range messages {
		candidates := messageContactCandidates(message)
		for _, candidate := range candidates {
			email := strings.ToLower(strings.TrimSpace(candidate.Email))
			if email == "" || email == selfEmail {
				continue
			}
			contact := contactsByEmail[email]
			if contact.ID == "" {
				_, domain := splitEmailAddress(email)
				contact = ContactRecord{
					ID:           contactID(email),
					Name:         firstNonEmpty(candidate.Name, emailName(email)),
					Email:        email,
					Avatar:       avatarInitial(firstNonEmpty(candidate.Name, email)),
					Role:         firstNonEmpty(candidate.Role, "邮件联系人"),
					Organization: firstNonEmpty(domain, "外部邮箱"),
					Note:         "基于真实邮件往来自动沉淀",
					Tags:         []string{},
					Stats:        ContactStats{},
				}
			}
			contact.Stats.TotalMessages += 1
			if message.Source == "template" || contains(message.Tags, "通知模板") {
				contact.Stats.InviteCount += 1
				contact.Tags = dedupeStrings(append(contact.Tags, "通知模板"))
			}
			for _, tag := range message.Tags {
				if tag != "" && tag != "已发送" && tag != "草稿" {
					contact.Tags = dedupeStrings(append(contact.Tags, tag))
				}
			}
			sortAt := firstNonEmpty(message.SortAt, message.SentAt, message.ReceivedAt)
			if contact.SortAt == "" || sortAt > contact.SortAt {
				contact.SortAt = sortAt
				contact.LastContactedAt = firstNonEmpty(message.DateTimeLabel, formatMailDateTimeLabel(sortAt))
				contact.Role = firstNonEmpty(candidate.Role, contact.Role)
				contact.Name = firstNonEmpty(candidate.Name, contact.Name)
				contact.Avatar = avatarInitial(firstNonEmpty(contact.Name, email))
			}
			contactsByEmail[email] = contact
		}
	}
	contacts := make([]ContactRecord, 0, len(contactsByEmail))
	needle := strings.ToLower(strings.TrimSpace(search))
	for _, contact := range contactsByEmail {
		if len(contact.Tags) == 0 {
			contact.Tags = []string{"邮件往来"}
		}
		if contact.LastContactedAt == "" {
			contact.LastContactedAt = "-"
		}
		if needle != "" && !contactMatchesSearch(contact, needle) {
			continue
		}
		contacts = append(contacts, contact)
	}
	sort.Slice(contacts, func(i, j int) bool {
		return contacts[i].SortAt > contacts[j].SortAt
	})
	return contacts
}

type messageContactCandidate struct {
	Name  string
	Email string
	Role  string
}

func messageContactCandidates(message MailMessage) []messageContactCandidate {
	candidates := []messageContactCandidate{}
	if strings.TrimSpace(message.SenderEmail) != "" {
		candidates = append(candidates, messageContactCandidate{
			Name:  firstNonEmpty(message.Sender, emailName(message.SenderEmail)),
			Email: message.SenderEmail,
			Role:  firstNonEmpty(message.Role, "邮件联系人"),
		})
	}
	for _, recipient := range message.Recipients {
		candidates = append(candidates, messageContactCandidate{
			Name:  emailName(recipient),
			Email: recipient,
			Role:  "邮件联系人",
		})
	}
	return candidates
}

func contactThreadItems(messages []MailMessage, contact ContactRecord) []ContactThreadItem {
	items := []ContactThreadItem{}
	for _, message := range messages {
		if !messageMatchesContact(message, contact) {
			continue
		}
		items = append(items, ContactThreadItem{
			MailMessage: message,
			FolderID:    firstNonEmpty(message.Folder, "inbox"),
		})
	}
	sort.Slice(items, func(i, j int) bool {
		return firstNonEmpty(items[i].SortAt, items[i].SentAt, items[i].ReceivedAt) > firstNonEmpty(items[j].SortAt, items[j].SentAt, items[j].ReceivedAt)
	})
	return items
}

func messageMatchesContact(message MailMessage, contact ContactRecord) bool {
	email := strings.ToLower(strings.TrimSpace(contact.Email))
	if email == "" {
		return false
	}
	if strings.EqualFold(message.SenderEmail, email) {
		return true
	}
	for _, recipient := range message.Recipients {
		if strings.EqualFold(recipient, email) {
			return true
		}
	}
	return false
}

func contactMatchesSearch(contact ContactRecord, needle string) bool {
	haystack := strings.ToLower(strings.Join([]string{
		contact.Name,
		contact.Email,
		contact.Role,
		contact.Organization,
		contact.Note,
		strings.Join(contact.Tags, " "),
	}, " "))
	return strings.Contains(haystack, needle)
}

func contactID(email string) string {
	hash := sha256.Sum256([]byte(strings.ToLower(strings.TrimSpace(email))))
	return "contact-" + hex.EncodeToString(hash[:])[:16]
}

func emailName(email string) string {
	local, _ := splitEmailAddress(email)
	if local == "" {
		return strings.TrimSpace(email)
	}
	return local
}

func buildMailTemplates(user UserRecord) []MailTemplate {
	copies := []struct {
		Role     string
		Subject  string
		Subtitle string
		Body     string
		Bullets  []string
		Action   string
	}{
		{
			Role:     "account",
			Subject:  "公司邮箱账号开通通知",
			Subtitle: "您的公司邮箱账号已准备开通",
			Body:     "管理员已为您预留公司邮箱账号。请按通知完成登录或注册，后续系统通知、验证码和重要邮件将统一进入公司邮箱。",
			Bullets:  []string{"账号由后台统一管理", "支持邮箱、手机号验证码或 OAuth 登录", "可按注册码完成注册"},
			Action:   "完成账号开通",
		},
		{
			Role:     "collaboration",
			Subject:  "公司邮箱协作邀请",
			Subtitle: "请通过悦享邮局完成后续邮件沟通",
			Body:     "我们邀请您使用本公司邮箱入口完成后续协作沟通。所有往来邮件都会沉淀到公司邮局，便于统一追踪、归档和审计。",
			Bullets:  []string{"统一公司邮箱身份", "邮件往来自动沉淀", "支持附件、草稿、星标和状态同步"},
			Action:   "进入邮箱协作",
		},
		{
			Role:     "notice",
			Subject:  "公司客服通知",
			Subtitle: "请关注此邮件中的处理事项",
			Body:     "这封邮件来自公司邮箱系统，用于同步客服、运营或内部协作事项。请直接回复本邮件，后续沟通会进入公司邮件归档。",
			Bullets:  []string{"可直接回复邮件", "沟通过程统一归档", "后台可追踪账号和投递状态"},
			Action:   "查看通知内容",
		},
	}
	templates := make([]MailTemplate, 0, len(copies))
	for _, copy := range copies {
		templates = append(templates, MailTemplate{
			ID:      "template-" + copy.Role,
			Role:    copy.Role,
			Subject: copy.Subject,
			HTML:    buildMailTemplateHTML(copy.Subject, copy.Subtitle, copy.Body, copy.Bullets, copy.Action, user),
		})
	}
	return templates
}

func buildMailTemplateHTML(subject string, subtitle string, body string, bullets []string, action string, user UserRecord) string {
	escapedBullets := make([]string, 0, len(bullets))
	for _, bullet := range bullets {
		escapedBullets = append(escapedBullets, "<li>"+htmlEscape(bullet)+"</li>")
	}
	return `
    <div class="border border-slate-200 rounded-xl overflow-hidden bg-white">
      <div class="h-2 bg-[#009BF5] w-full"></div>
      <div class="p-8">
        <div class="text-2xl font-bold text-slate-900 mb-2">` + htmlEscape(subject) + `</div>
        <div class="text-slate-500 mb-8">` + htmlEscape(subtitle) + `</div>
        <div class="space-y-4 text-slate-700 text-sm leading-relaxed mb-8">
          <p>您好：</p>
          <p>` + htmlEscape(body) + `</p>
          <ul class="list-disc pl-5 space-y-1 text-slate-600">` + strings.Join(escapedBullets, "") + `</ul>
        </div>
        <div class="text-center">
          <span class="inline-block bg-[#009BF5] text-white px-8 py-3 rounded-full font-medium">` + htmlEscape(action) + `</span>
        </div>
      </div>
      <div class="bg-slate-50 p-4 text-center text-xs text-slate-400 border-t border-slate-100">
        此邮件由悦享邮局发送 · 发起人 ` + htmlEscape(firstNonEmpty(user.DisplayName, user.Email, "公司用户")) + `
      </div>
    </div>
  `
}

func selectMailTemplate(templates []MailTemplate, role string) MailTemplate {
	role = strings.ToLower(strings.TrimSpace(role))
	for _, template := range templates {
		if template.Role == role || template.ID == role {
			return template
		}
	}
	if len(templates) == 0 {
		return MailTemplate{ID: "template-account", Role: "account", Subject: "公司邮箱账号开通通知", HTML: "<p>欢迎使用悦享邮局</p>"}
	}
	return templates[0]
}

func resolveInboundAccountIndex(users []UserRecord, payload InboundMailPayload) (int, error) {
	if accountID := strings.TrimSpace(payload.AccountID); accountID != "" {
		for index, user := range users {
			if user.ID == accountID && accountActive(user.RegisteredUser) {
				return index, nil
			}
		}
	}
	candidates := inboundRecipientCandidates(payload)
	for index, user := range users {
		if !accountActive(user.RegisteredUser) {
			continue
		}
		for _, candidate := range candidates {
			if strings.EqualFold(candidate, user.Email) {
				return index, nil
			}
		}
	}
	return -1, errors.New("收信 Webhook 未找到匹配的有效账号")
}

func inboundRecipientCandidates(payload InboundMailPayload) []string {
	candidates := []string{}
	if email := strings.ToLower(strings.TrimSpace(payload.Email)); email != "" {
		candidates = append(candidates, email)
	}
	local := normalizeMailboxName(payload.LocalPart)
	domain := normalizeDomain(payload.Domain)
	if local != "" && domain != "" {
		candidates = append(candidates, local+"@"+domain)
	}
	for _, value := range append(append([]string{}, payload.To...), payload.Recipients...) {
		addresses, err := netmail.ParseAddressList(value)
		if err == nil {
			for _, address := range addresses {
				candidates = append(candidates, strings.ToLower(strings.TrimSpace(address.Address)))
			}
			continue
		}
		if strings.Contains(value, "@") {
			candidates = append(candidates, strings.ToLower(strings.TrimSpace(value)))
		}
	}
	return dedupeStrings(candidates)
}

func buildInboundMailMessage(user UserRecord, payload InboundMailPayload) (MailMessage, error) {
	if len(payload.Raw) > 0 && string(payload.Raw) != "null" {
		message := parseMailMessageRaw(payload.Raw)
		message = normalizeRemoteMailMessage(message, user, "inbox", time.Now())
		if strings.TrimSpace(message.ID) == "" {
			message.ID = stableRemoteMessageID(message)
		}
		return message, nil
	}
	fromName, fromEmail := parseMailboxAddress(firstNonEmpty(payload.From, payload.SenderEmail))
	sender := firstNonEmpty(payload.Sender, fromName, emailName(fromEmail), "未知发件人")
	senderEmail := strings.ToLower(firstNonEmpty(payload.SenderEmail, fromEmail))
	if senderEmail == "" {
		return MailMessage{}, errors.New("收信 Webhook 缺少发件人邮箱")
	}
	recipients, err := normalizeEmailList(inboundRecipientCandidates(payload), false)
	if err != nil {
		return MailMessage{}, err
	}
	if len(recipients) == 0 {
		recipients = []string{user.Email}
	}
	receivedAt := firstNonEmpty(payload.ReceivedAt, nowISO())
	body := normalizeMessageBody(payload.Body)
	body.Text = firstNonEmpty(body.Text, payload.Text)
	body.HTML = firstNonEmpty(body.HTML, payload.HTML)
	body = normalizeMessageBody(body)
	attachments, err := normalizeAttachmentList(payload.Attachments)
	if err != nil {
		return MailMessage{}, err
	}
	subject := truncateRunes(firstNonEmpty(payload.Subject, "(无主题)"), 240)
	content := composeBodyHTML(body, subject)
	snippet := truncateRunes(firstNonEmpty(payload.Snippet, body.Text, stripHTML(content), subject), 120)
	message := MailMessage{
		ID:                firstNonEmpty(payload.MessageID, payload.ProviderMessageID),
		ThreadID:          strings.TrimSpace(payload.ThreadID),
		Folder:            "inbox",
		PreviousFolder:    "inbox",
		Sender:            sender,
		SenderEmail:       senderEmail,
		Recipients:        recipients,
		Avatar:            avatarInitial(sender),
		Role:              "邮件联系人",
		Subject:           subject,
		Snippet:           snippet,
		Time:              formatMailTimeLabel(receivedAt),
		DateTimeLabel:     formatMailDateTimeLabel(receivedAt),
		SortAt:            receivedAt,
		ReceivedAt:        receivedAt,
		IsUnread:          true,
		IsStarred:         false,
		HasAttachment:     len(attachments) > 0,
		Tags:              []string{"收件"},
		IsOutgoing:        false,
		Content:           content,
		Attachments:       attachments,
		Source:            "mailbox",
		DeliveryStatus:    "received",
		ProviderMessageID: strings.TrimSpace(payload.ProviderMessageID),
	}
	if strings.TrimSpace(message.ID) == "" {
		message.ID = stableRemoteMessageID(message)
	}
	return message, nil
}

func parseMailboxAddress(value string) (string, string) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", ""
	}
	address, err := netmail.ParseAddress(value)
	if err != nil {
		if strings.Contains(value, "@") {
			return "", strings.ToLower(value)
		}
		return value, ""
	}
	return strings.TrimSpace(address.Name), strings.ToLower(strings.TrimSpace(address.Address))
}

func updateStoredDeliveryStatus(state *AppState, payload DeliveryStatusPayload) (MailMessage, bool) {
	status := normalizeDeliveryStatus(payload.Status)
	deliveryError := firstNonEmpty(payload.DeliveryError, payload.Error)
	for accountID, messages := range state.Messages {
		for index := range messages {
			message := &state.Messages[accountID][index]
			if !deliveryStatusMatchesMessage(*message, payload) {
				continue
			}
			message.DeliveryStatus = status
			message.DeliveryError = deliveryError
			if payload.AcceptedAt != "" {
				message.AcceptedAt = payload.AcceptedAt
			}
			if payload.ProviderMessageID != "" {
				message.ProviderMessageID = payload.ProviderMessageID
			}
			return *message, true
		}
	}
	return MailMessage{}, false
}

func deliveryStatusMatchesMessage(message MailMessage, payload DeliveryStatusPayload) bool {
	messageID := strings.TrimSpace(payload.MessageID)
	providerID := strings.TrimSpace(payload.ProviderMessageID)
	return (messageID != "" && message.ID == messageID) ||
		(providerID != "" && message.ProviderMessageID == providerID)
}

func normalizeDeliveryStatus(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "queued", "accepted", "sent", "delivered", "received", "failed", "bounced", "rejected":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return "accepted"
	}
}

func inviteRoleLabel(role string) string {
	switch strings.ToLower(strings.TrimSpace(role)) {
	case "account":
		return "账号开通"
	case "collaboration":
		return "协作邀请"
	case "notice":
		return "客服通知"
	default:
		return "通知模板"
	}
}

func dedupeStrings(values []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

func filterMessages(messages []MailMessage, folderID string, filter string, search string) []MailMessage {
	items := make([]MailMessage, 0, len(messages))
	for _, message := range messages {
		if folderID == "starred" {
			if !message.IsStarred || message.Folder == "trash" {
				continue
			}
		} else if message.Folder != folderID {
			continue
		}
		if filter == "unread" && !message.IsUnread {
			continue
		}
		if filter == "important" && !message.IsStarred {
			continue
		}
		if filter == "attachment" && !message.HasAttachment {
			continue
		}
		if search != "" {
			haystack := strings.ToLower(strings.Join([]string{message.Sender, message.SenderEmail, message.Subject, message.Snippet, strings.Join(message.Tags, " ")}, " "))
			if !strings.Contains(haystack, search) {
				continue
			}
		}
		items = append(items, message)
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].SortAt > items[j].SortAt
	})
	return items
}

func folderCounts(messages []MailMessage) response {
	counts := response{"inbox": 0, "starred": 0, "sent": 0, "drafts": 0, "trash": 0, "archive": 0}
	for _, message := range messages {
		if message.Folder == "inbox" && message.IsUnread {
			counts["inbox"] = counts["inbox"].(int) + 1
		}
		if message.IsStarred && message.Folder != "trash" {
			counts["starred"] = counts["starred"].(int) + 1
		}
		if _, ok := counts[message.Folder]; ok && message.Folder != "inbox" {
			counts[message.Folder] = counts[message.Folder].(int) + 1
		}
	}
	return counts
}

func defaultSettings(user UserRecord) MailSettings {
	return MailSettings{
		DefaultSenderName: "悦享用户_" + firstNonEmpty(user.DisplayName, "MyName"),
		Signature:         "--\n此致\n悦享邮局",
		AutoReplyEnabled:  false,
		AutoReplyMessage:  "您好，我暂时无法及时回复，稍后会第一时间处理您的来信。",
	}
}

func timeToISO(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.Format(time.RFC3339)
}

func nullTimeToISO(value sql.NullTime) string {
	if !value.Valid {
		return ""
	}
	return timeToISO(value.Time)
}

func timeFromISOOrNow(value string) time.Time {
	if parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(value)); err == nil {
		return parsed
	}
	return time.Now()
}

func timeFromISOOrNil(value string) any {
	if parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(value)); err == nil {
		return parsed
	}
	return nil
}

func nonNilStrings(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}

func nonNilAnys(values []any) []any {
	if values == nil {
		return []any{}
	}
	return values
}

func hashPassword(password string) (string, error) {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	const memory = 64 * 1024
	const iterations = 3
	const parallelism = 2
	const keyLength = 32
	hash := argon2.IDKey([]byte(password), salt, iterations, memory, parallelism, keyLength)
	return fmt.Sprintf(
		"argon2id:v=19:m=%d,t=%d,p=%d:%s:%s",
		memory,
		iterations,
		parallelism,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(hash),
	), nil
}

func verifyPassword(encoded string, password string) bool {
	if strings.HasPrefix(encoded, "argon2id:") {
		return verifyArgon2idPassword(encoded, password)
	}
	parts := strings.Split(encoded, ":")
	if len(parts) != 3 || parts[0] != "sha256" {
		return false
	}
	hash := sha256.Sum256([]byte(parts[1] + ":" + password))
	expected := []byte(parts[2])
	actual := []byte(hex.EncodeToString(hash[:]))
	return subtle.ConstantTimeCompare(expected, actual) == 1
}

func verifyArgon2idPassword(encoded string, password string) bool {
	parts := strings.Split(encoded, ":")
	if len(parts) != 5 || parts[0] != "argon2id" || parts[1] != "v=19" {
		return false
	}
	params := map[string]uint32{}
	for _, pair := range strings.Split(parts[2], ",") {
		name, value, ok := strings.Cut(pair, "=")
		if !ok {
			return false
		}
		parsed, err := strconv.ParseUint(value, 10, 32)
		if err != nil {
			return false
		}
		params[name] = uint32(parsed)
	}
	memory := params["m"]
	iterations := params["t"]
	parallelism := uint8(params["p"])
	if memory == 0 || iterations == 0 || parallelism == 0 {
		return false
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[3])
	if err != nil {
		return false
	}
	expected, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return false
	}
	actual := argon2.IDKey([]byte(password), salt, iterations, memory, parallelism, uint32(len(expected)))
	return subtle.ConstantTimeCompare(expected, actual) == 1
}

func envelope(data any) response {
	return response{"success": true, "data": data}
}

func badRequest(message string) (int, any) {
	return http.StatusBadRequest, response{"message": message}
}

func unauthorized() (int, any) {
	return http.StatusUnauthorized, response{"message": "请先登录"}
}

func domainError(err error) (int, any) {
	if errors.Is(err, errUnauthorized) {
		return unauthorized()
	}
	return http.StatusBadRequest, response{"message": err.Error()}
}

func serverError(err error) (int, any) {
	slog.Error("request failed", "error", err)
	return http.StatusInternalServerError, response{"message": "server error"}
}

func jsonHandler(fn apiHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		status, payload := fn(r.WithContext(context.WithValue(r.Context(), responseWriterKey{}, w)))
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(status)
		if err := json.NewEncoder(w).Encode(payload); err != nil {
			slog.Warn("json response failed", "error", err)
		}
	}
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		slog.Warn("json response failed", "error", err)
	}
}

func withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
		}
		w.Header().Set("Access-Control-Allow-Credentials", "true")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Admin-Token")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, PUT, DELETE, OPTIONS")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func withRequestLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		slog.Info("http request", "method", r.Method, "path", r.URL.Path, "duration_ms", time.Since(start).Milliseconds())
	})
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func envBool(key string, fallback bool) bool {
	value := strings.ToLower(strings.TrimSpace(os.Getenv(key)))
	if value == "" {
		return fallback
	}
	switch value {
	case "1", "true", "yes", "y", "on":
		return true
	case "0", "false", "no", "n", "off":
		return false
	default:
		return fallback
	}
}

func productionStrictEnabled() bool {
	return envBool("INFINITEMAIL_PRODUCTION_STRICT", false) || envBool("PRODUCTION_STRICT", false)
}

func requirePostgresStore() bool {
	return productionStrictEnabled() || envBool("REQUIRE_POSTGRES", false)
}

func requireRealSMSProvider() bool {
	return productionStrictEnabled() || envBool("REQUIRE_REAL_SMS", false)
}

func requireRealOAuthProvider() bool {
	return productionStrictEnabled() || envBool("REQUIRE_REAL_OAUTH", false)
}

func requireMailWebhook() bool {
	return envBool("REQUIRE_MAIL_WEBHOOK", false)
}

func mailServerStrictDataPlane(config MailServerConfig) bool {
	return config.StrictDataPlane || productionStrictEnabled() || envBool("REQUIRE_MAIL_DATA_PLANE", false)
}

func storeKindFromEnv() string {
	if strings.TrimSpace(env("DATABASE_URL", "")) != "" {
		return "postgres"
	}
	return "json"
}

func normalizeDomain(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.TrimPrefix(value, "@")
	if value == "" {
		return "yuexiang.com"
	}
	return value
}

func normalizeAllowedPrefixes(values []string) []string {
	seen := map[string]bool{}
	prefixes := []string{}
	for _, value := range values {
		prefix := normalizePrefix(value)
		if prefix == "" || seen[prefix] {
			continue
		}
		seen[prefix] = true
		prefixes = append(prefixes, prefix)
	}
	if len(prefixes) == 0 {
		return []string{"user"}
	}
	return prefixes
}

func resolveDefaultPrefix(config MailboxConfig) string {
	configured := normalizePrefix(config.DefaultPrefix)
	allowed := normalizeAllowedPrefixes(config.AllowedPrefixes)
	if contains(allowed, configured) {
		return configured
	}
	return allowed[0]
}

func defaultRolePrefix(config MailboxConfig) string {
	if !config.PrefixPolicyEnabled {
		return ""
	}
	return resolveDefaultPrefix(config) + "-"
}

func normalizePrefix(value string) string {
	return prefixRe.ReplaceAllString(strings.ToLower(strings.TrimSpace(strings.TrimSuffix(value, "-"))), "")
}

func normalizeMailboxName(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = localPartRe.ReplaceAllString(value, "")
	value = strings.Trim(value, "._-")
	return value
}

func normalizePhone(value string) string {
	replacer := strings.NewReplacer(" ", "", "-", "", "(", "", ")", "", "+86", "")
	return replacer.Replace(strings.TrimSpace(value))
}

func maskPhone(value string) string {
	if len(value) != 11 {
		return value
	}
	return value[:3] + "****" + value[7:]
}

func maskSecret(value string) string {
	value = strings.TrimSpace(value)
	if len(value) <= 6 {
		return "******"
	}
	return value[:3] + "******" + value[len(value)-3:]
}

func truncateString(value string, maxLength int) string {
	value = strings.TrimSpace(value)
	if maxLength <= 0 || len(value) <= maxLength {
		return value
	}
	return value[:maxLength]
}

func deviceLabelFromUserAgent(userAgent string) string {
	userAgent = strings.TrimSpace(userAgent)
	if userAgent == "" {
		return "未知设备"
	}
	lower := strings.ToLower(userAgent)
	osName := "未知系统"
	switch {
	case strings.Contains(lower, "iphone"):
		osName = "iPhone"
	case strings.Contains(lower, "ipad"):
		osName = "iPad"
	case strings.Contains(lower, "android"):
		osName = "Android"
	case strings.Contains(lower, "mac os x") || strings.Contains(lower, "macintosh"):
		osName = "macOS"
	case strings.Contains(lower, "windows"):
		osName = "Windows"
	case strings.Contains(lower, "linux"):
		osName = "Linux"
	}
	appName := "浏览器"
	switch {
	case strings.Contains(lower, "lark") || strings.Contains(lower, "feishu"):
		appName = "飞书"
	case strings.Contains(lower, "micromessenger"):
		appName = "微信"
	case strings.Contains(lower, "edg/"):
		appName = "Edge"
	case strings.Contains(lower, "firefox/"):
		appName = "Firefox"
	case strings.Contains(lower, "chrome/") || strings.Contains(lower, "crios/"):
		appName = "Chrome"
	case strings.Contains(lower, "safari/"):
		appName = "Safari"
	}
	if osName == "未知系统" && appName == "浏览器" {
		return "未知设备"
	}
	return osName + " · " + appName
}

func sourceLabel(invited bool, fallback string) string {
	if invited {
		return "invite"
	}
	return fallback
}

func sessionExpired(session SessionRecord) bool {
	expiresAt, err := time.Parse(time.RFC3339, session.ExpiresAt)
	return err != nil || time.Now().After(expiresAt)
}

func nowISO() string {
	return time.Now().Format(time.RFC3339)
}

func nextID(prefix string) string {
	return prefix + "-" + strings.ToLower(randomBase32(8))
}

func randomDigits(length int) string {
	digits := "0123456789"
	var builder strings.Builder
	for builder.Len() < length {
		bytes := make([]byte, 1)
		if _, err := rand.Read(bytes); err != nil {
			builder.WriteByte('0')
			continue
		}
		builder.WriteByte(digits[int(bytes[0])%len(digits)])
	}
	return builder.String()
}

func randomBase32(length int) string {
	alphabet := "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	var builder strings.Builder
	for builder.Len() < length {
		bytes := make([]byte, 1)
		if _, err := rand.Read(bytes); err != nil {
			builder.WriteByte('A')
			continue
		}
		builder.WriteByte(alphabet[int(bytes[0])%len(alphabet)])
	}
	return builder.String()
}

func randomHex(byteCount int) string {
	bytes := make([]byte, byteCount)
	if _, err := rand.Read(bytes); err != nil {
		fallback := sha256.Sum256([]byte(fmt.Sprintf("%d", time.Now().UnixNano())))
		return hex.EncodeToString(fallback[:])
	}
	return hex.EncodeToString(bytes)
}

func contains(items []string, value string) bool {
	for _, item := range items {
		if item == value {
			return true
		}
	}
	return false
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func stringFromAny(value any) string {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case float64:
		return strings.TrimSpace(strconv.FormatFloat(typed, 'f', -1, 64))
	case int:
		return strings.TrimSpace(strconv.Itoa(typed))
	case fmt.Stringer:
		return strings.TrimSpace(typed.String())
	default:
		return ""
	}
}

func boolFromAny(value any) bool {
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		switch strings.ToLower(strings.TrimSpace(typed)) {
		case "true", "1", "yes", "y":
			return true
		default:
			return false
		}
	case float64:
		return typed != 0
	default:
		return false
	}
}

func stringSliceFromAny(value any) []string {
	switch typed := value.(type) {
	case []string:
		return nonNilStrings(typed)
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			if text := stringFromAny(item); text != "" {
				out = append(out, text)
			}
		}
		return out
	case string:
		if strings.TrimSpace(typed) == "" {
			return nil
		}
		return []string{strings.TrimSpace(typed)}
	default:
		return nil
	}
}

func anySliceFromAny(value any) []any {
	switch typed := value.(type) {
	case []any:
		return nonNilAnys(typed)
	default:
		return nil
	}
}

func mapFromAny(value any) map[string]any {
	switch typed := value.(type) {
	case map[string]any:
		if typed == nil {
			return map[string]any{}
		}
		return typed
	default:
		return map[string]any{}
	}
}

func firstRune(value string) string {
	value = strings.TrimSpace(value)
	for _, item := range value {
		return string(item)
	}
	return "M"
}
