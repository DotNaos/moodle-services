package moodleservice

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/DotNaos/moodle-services/internal/moodle"
)

func MobileSessionFromCredentials(ctx context.Context, credentials WebexCredentials) (moodle.MobileSession, moodle.MobileSiteInfo, error) {
	if strings.TrimSpace(credentials.Username) == "" || strings.TrimSpace(credentials.Password) == "" {
		return moodle.MobileSession{}, moodle.MobileSiteInfo{}, fmt.Errorf("username and password are required")
	}
	ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()

	school := moodle.GetDefaultSchool()
	browser := newWebBrowser()
	if _, err := browser.loginFHGR(ctx, school.MoodleURL, "/my/", credentials); err != nil {
		return moodle.MobileSession{}, moodle.MobileSiteInfo{}, err
	}

	launchURL, err := moodle.BuildMobileLaunchURL(
		school.MoodleURL,
		moodle.DefaultMobileLaunchService,
		"",
		moodle.DefaultMobileLaunchScheme,
	)
	if err != nil {
		return moodle.MobileSession{}, moodle.MobileSiteInfo{}, err
	}
	page, err := browser.request(ctx, http.MethodGet, launchURL.URL, "", map[string]string{
		"User-Agent": "Mozilla/5.0 MoodleMobile",
	}, nil)
	if err != nil {
		return moodle.MobileSession{}, moodle.MobileSiteInfo{}, err
	}

	callbackURL := nonEmpty(redirectURL(page), htmlRedirect(page.text, page.url))
	if callbackURL == "" {
		return moodle.MobileSession{}, moodle.MobileSiteInfo{}, fmt.Errorf("mobile launch login did not return a token")
	}
	callback, err := moodle.ParseMobileLaunchCallback(callbackURL)
	if err != nil {
		return moodle.MobileSession{}, moodle.MobileSiteInfo{}, err
	}
	if callback.SiteID != launchURL.SiteID {
		return moodle.MobileSession{}, moodle.MobileSiteInfo{}, fmt.Errorf("mobile launch callback site id mismatch")
	}

	token := moodle.MobileTokenFromLaunch(launchURL.SiteURL, callback)
	siteInfo, err := moodle.FetchMobileSiteInfo(token)
	if err != nil {
		return moodle.MobileSession{}, moodle.MobileSiteInfo{}, err
	}
	token.UserID = siteInfo.UserID
	session := moodle.MobileSessionFromToken(token)
	session.SchoolID = moodle.ActiveSchoolID
	return session, siteInfo, nil
}
