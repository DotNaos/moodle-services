package moodleservices

import (
	"context"
	"net/http"

	"github.com/DotNaos/moodle-services/internal/auth"
	appcrypto "github.com/DotNaos/moodle-services/internal/crypto"
	"github.com/DotNaos/moodle-services/internal/httpapi"
	"github.com/DotNaos/moodle-services/internal/moodle"
	"github.com/DotNaos/moodle-services/internal/moodleservice"
	"github.com/DotNaos/moodle-services/internal/store"
)

var (
	ErrNotFound     = store.ErrNotFound
	ErrUnauthorized = auth.ErrUnauthorized
)

const ActiveSchoolID = moodle.ActiveSchoolID
const DefaultMobileLaunchService = moodle.DefaultMobileLaunchService
const DefaultMobileLaunchScheme = moodle.DefaultMobileLaunchScheme

type (
	APIKeyRecord                      = store.APIKeyRecord
	Box                               = appcrypto.Box
	DataClient                        = moodleservice.DataClient
	LoginOptions                      = moodle.LoginOptions
	LoginResult                       = moodle.LoginResult
	MobileLaunchOptions               = moodle.MobileLaunchOptions
	MobileLaunchResult                = moodle.MobileLaunchResult
	MobileLaunchToken                 = moodle.MobileLaunchToken
	MobileLaunchURL                   = moodle.MobileLaunchURL
	MobileClient                      = moodle.MobileClient
	MobileQRLink                      = moodle.MobileQRLink
	MobileToken                       = moodle.MobileToken
	MobileSession                     = moodle.MobileSession
	MoodleCredentials                 = store.MoodleCredentials
	OAuthAuthorizationCode            = store.OAuthAuthorizationCode
	OAuthClient                       = store.OAuthClient
	OAuthToken                        = store.OAuthToken
	CreateOAuthAuthorizationCodeInput = store.CreateOAuthAuthorizationCodeInput
	CreateOAuthClientInput            = store.CreateOAuthClientInput
	CreateOAuthTokenInput             = store.CreateOAuthTokenInput
	CreateMobileBridgeRequestInput    = store.CreateMobileBridgeRequestInput
	CodexStateSnapshot                = store.CodexStateSnapshot
	CodexStateSnapshotData            = store.CodexStateSnapshotData
	CreateCodexStateSnapshotInput     = store.CreateCodexStateSnapshotInput
	UpsertWebexSessionInput           = store.UpsertWebexSessionInput
	WebexCredentials                  = moodleservice.WebexCredentials
	MobileBridgeRequest               = store.MobileBridgeRequest
	Service                           = moodleservice.Service
	Store                             = store.Store
	UpsertMoodleAccountInput          = store.UpsertMoodleAccountInput
	User                              = store.User
)

func APIKeyFromRequest(r *http.Request) string {
	return auth.APIKeyFromRequest(r)
}

func GenerateAPIKey() (string, error) {
	return auth.GenerateAPIKey()
}

func HashAPIKey(key string) string {
	return auth.HashAPIKey(key)
}

func ConstantTimeEqual(left string, right string) bool {
	return auth.ConstantTimeEqual(left, right)
}

func NewBox(secret string) (Box, error) {
	return appcrypto.NewBox(secret)
}

func OpenStore(databaseURL string) (*Store, error) {
	return store.Open(databaseURL)
}

func ParseMobileQRLink(raw string) (MobileQRLink, error) {
	return moodle.ParseMobileQRLink(raw)
}

func ExchangeMobileQRToken(link MobileQRLink) (MobileToken, error) {
	return moodle.ExchangeMobileQRToken(link)
}

func MobileSessionFromToken(token MobileToken) MobileSession {
	return moodle.MobileSessionFromToken(token)
}

func BuildMobileLaunchURL(siteURL string, service string, passport string, urlScheme string) (MobileLaunchURL, error) {
	return moodle.BuildMobileLaunchURL(siteURL, service, passport, urlScheme)
}

func NewMobileLaunchPassport() (string, error) {
	return moodle.NewMobileLaunchPassport()
}

func ExpectedMobileLaunchSiteID(siteURL string, passport string) string {
	return moodle.ExpectedMobileLaunchSiteID(siteURL, passport)
}

func ParseMobileLaunchCallback(raw string) (MobileLaunchToken, error) {
	return moodle.ParseMobileLaunchCallback(raw)
}

func MobileTokenFromLaunch(siteURL string, launch MobileLaunchToken) MobileToken {
	return moodle.MobileTokenFromLaunch(siteURL, launch)
}

func LoginWithPlaywright(options LoginOptions) (LoginResult, error) {
	return moodle.LoginWithPlaywright(options)
}

func LaunchMobileLoginWithSession(options MobileLaunchOptions) (MobileLaunchResult, error) {
	return moodle.LaunchMobileLoginWithSession(options)
}

func FetchMobileSiteInfo(token MobileToken) (moodle.MobileSiteInfo, error) {
	return moodle.FetchMobileSiteInfo(token)
}

func GetDefaultSchool() moodle.SchoolConfig {
	return moodle.GetDefaultSchool()
}

func MobileSessionFromCredentials(ctx context.Context, credentials WebexCredentials) (MobileSession, moodle.MobileSiteInfo, error) {
	return moodleservice.MobileSessionFromCredentials(ctx, credentials)
}

func NewMobileClient(session MobileSession, schoolID string) (*MobileClient, error) {
	return moodle.NewMobileClient(session, schoolID)
}

func OpenAPISpecJSON() []byte {
	return httpapi.OpenAPISpecJSON()
}
