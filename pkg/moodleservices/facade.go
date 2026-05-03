package moodleservices

import (
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

type (
	APIKeyRecord                      = store.APIKeyRecord
	Box                               = appcrypto.Box
	DataClient                        = moodleservice.DataClient
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

func NewMobileClient(session MobileSession, schoolID string) (*MobileClient, error) {
	return moodle.NewMobileClient(session, schoolID)
}

func OpenAPISpecJSON() []byte {
	return httpapi.OpenAPISpecJSON()
}
