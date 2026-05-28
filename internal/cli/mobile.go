package cli

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/DotNaos/moodle-services/internal/moodle"
	"github.com/spf13/cobra"
)

type mobileQRInspectResult struct {
	Kind                 string `json:"kind" yaml:"kind"`
	SiteURL              string `json:"siteUrl" yaml:"siteUrl"`
	UserID               int    `json:"userId,omitempty" yaml:"userId,omitempty"`
	QRLoginKeyRedacted   string `json:"qrLoginKeyRedacted,omitempty" yaml:"qrLoginKeyRedacted,omitempty"`
	PublicConfigEndpoint string `json:"publicConfigEndpoint" yaml:"publicConfigEndpoint"`
	TokenEndpoint        string `json:"tokenEndpoint,omitempty" yaml:"tokenEndpoint,omitempty"`
	TokenWSFunction      string `json:"tokenWsFunction,omitempty" yaml:"tokenWsFunction,omitempty"`
	SampleTokenRequest   string `json:"sampleTokenRequest,omitempty" yaml:"sampleTokenRequest,omitempty"`
	SafetyNote           string `json:"safetyNote" yaml:"safetyNote"`
}

type mobileQRLoginResult struct {
	Status                string                `json:"status" yaml:"status"`
	SiteURL               string                `json:"siteUrl" yaml:"siteUrl"`
	UserID                int                   `json:"userId" yaml:"userId"`
	MobileSessionPath     string                `json:"mobileSessionPath,omitempty" yaml:"mobileSessionPath,omitempty"`
	QRLoginKeyRedacted    string                `json:"qrLoginKeyRedacted" yaml:"qrLoginKeyRedacted"`
	TokenReceived         bool                  `json:"tokenReceived" yaml:"tokenReceived"`
	PrivateTokenReceived  bool                  `json:"privateTokenReceived" yaml:"privateTokenReceived"`
	TokenRedacted         string                `json:"tokenRedacted,omitempty" yaml:"tokenRedacted,omitempty"`
	PrivateTokenRedacted  string                `json:"privateTokenRedacted,omitempty" yaml:"privateTokenRedacted,omitempty"`
	SiteName              string                `json:"siteName,omitempty" yaml:"siteName,omitempty"`
	Username              string                `json:"username,omitempty" yaml:"username,omitempty"`
	MobileFunctionChecked string                `json:"mobileFunctionChecked,omitempty" yaml:"mobileFunctionChecked,omitempty"`
	CourseCount           int                   `json:"courseCount,omitempty" yaml:"courseCount,omitempty"`
	SampleCourses         []mobileCourseSummary `json:"sampleCourses,omitempty" yaml:"sampleCourses,omitempty"`
	SafetyNote            string                `json:"safetyNote" yaml:"safetyNote"`
}

type mobileLaunchLoginResult struct {
	Status                string                `json:"status" yaml:"status"`
	SiteURL               string                `json:"siteUrl" yaml:"siteUrl"`
	UserID                int                   `json:"userId" yaml:"userId"`
	Username              string                `json:"username,omitempty" yaml:"username,omitempty"`
	SiteName              string                `json:"siteName,omitempty" yaml:"siteName,omitempty"`
	LaunchURL             string                `json:"launchUrl,omitempty" yaml:"launchUrl,omitempty"`
	URLScheme             string                `json:"urlScheme" yaml:"urlScheme"`
	PassportRedacted      string                `json:"passportRedacted" yaml:"passportRedacted"`
	SiteIDMatched         bool                  `json:"siteIdMatched" yaml:"siteIdMatched"`
	MobileSessionPath     string                `json:"mobileSessionPath,omitempty" yaml:"mobileSessionPath,omitempty"`
	TokenReceived         bool                  `json:"tokenReceived" yaml:"tokenReceived"`
	PrivateTokenReceived  bool                  `json:"privateTokenReceived" yaml:"privateTokenReceived"`
	TokenRedacted         string                `json:"tokenRedacted,omitempty" yaml:"tokenRedacted,omitempty"`
	PrivateTokenRedacted  string                `json:"privateTokenRedacted,omitempty" yaml:"privateTokenRedacted,omitempty"`
	MobileFunctionChecked string                `json:"mobileFunctionChecked,omitempty" yaml:"mobileFunctionChecked,omitempty"`
	CourseCount           int                   `json:"courseCount,omitempty" yaml:"courseCount,omitempty"`
	SampleCourses         []mobileCourseSummary `json:"sampleCourses,omitempty" yaml:"sampleCourses,omitempty"`
	SafetyNote            string                `json:"safetyNote" yaml:"safetyNote"`
}

type mobileCourseSummary struct {
	ID        int    `json:"id" yaml:"id"`
	FullName  string `json:"fullname" yaml:"fullname"`
	ShortName string `json:"shortname" yaml:"shortname"`
}

var mobileQRLoginSkipCheck bool
var mobileLaunchSite string
var mobileLaunchService string
var mobileLaunchPassport string
var mobileLaunchScheme string
var mobileLaunchCallback string
var mobileLaunchHeaded bool
var mobileLaunchSkipCourses bool
var mobileLaunchTimeout time.Duration

var mobileCmd = &cobra.Command{
	Use:   "mobile",
	Short: "Inspect Moodle mobile app links",
	Long:  "Inspect Moodle mobile app links and explain which Moodle Mobile web services they use.",
}

var mobileQRCmd = &cobra.Command{
	Use:   "qr",
	Short: "Inspect Moodle mobile QR login links",
}

var mobileQRInspectCmd = &cobra.Command{
	Use:   "inspect <moodlemobile-link>",
	Short: "Explain a Moodle mobile QR login link without redeeming it",
	Long:  "Explain a Moodle mobile QR login link without redeeming it. This does not contact Moodle or consume the one-time QR login key.",
	Args:  cobra.ExactArgs(1),
	ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return nil, cobra.ShellCompDirectiveNoFileComp
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		result, err := inspectMobileQRLink(args[0])
		if err != nil {
			return err
		}
		return writeCommandOutput(cmd, result, func(w io.Writer) error {
			return renderMobileQRInspectText(w, result)
		})
	},
}

var mobileQRLoginCmd = &cobra.Command{
	Use:   "login",
	Short: "Create a Moodle mobile token from a freshly scraped profile QR code",
	Long:  "Load your Moodle profile with the saved web session, decode the mobile app QR code, exchange it for a Moodle mobile token, and verify it with read-only mobile API calls.",
	Args:  cobra.NoArgs,
	ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return nil, cobra.ShellCompDirectiveNoFileComp
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		result, err := runMobileQRLogin(!mobileQRLoginSkipCheck)
		if err != nil {
			return err
		}
		return writeCommandOutput(cmd, result, func(w io.Writer) error {
			return renderMobileQRLoginText(w, result)
		})
	},
}

var mobileLaunchLoginCmd = &cobra.Command{
	Use:     "launch-login [callback-url]",
	Aliases: []string{"browser-login"},
	Short:   "Create a Moodle mobile token through the Moodle App browser launch flow",
	Long:    "Open Moodle's mobile launch endpoint with the saved web session, capture the returned deep-link token, save it as the local mobile session, and verify it with read-only mobile API calls. A callback URL can also be passed manually.",
	Args:    cobra.MaximumNArgs(1),
	ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return nil, cobra.ShellCompDirectiveNoFileComp
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		callback := mobileLaunchCallback
		if len(args) == 1 {
			callback = args[0]
		}
		result, err := runMobileLaunchLogin(mobileLaunchLoginOptions{
			SiteURL:      mobileLaunchSite,
			Service:      mobileLaunchService,
			Passport:     mobileLaunchPassport,
			URLScheme:    mobileLaunchScheme,
			CallbackURL:  callback,
			Headless:     !mobileLaunchHeaded,
			CheckCourses: !mobileLaunchSkipCourses,
			Timeout:      mobileLaunchTimeout,
		})
		if err != nil {
			return err
		}
		return writeCommandOutput(cmd, result, func(w io.Writer) error {
			return renderMobileLaunchLoginText(w, result)
		})
	},
}

func init() {
	mobileQRLoginCmd.Flags().BoolVar(&mobileQRLoginSkipCheck, "skip-check", false, "Create the mobile token but skip read-only API verification")
	mobileLaunchLoginCmd.Flags().StringVar(&mobileLaunchSite, "site", moodle.GetDefaultSchool().MoodleURL, "Moodle site URL")
	mobileLaunchLoginCmd.Flags().StringVar(&mobileLaunchService, "service", moodle.DefaultMobileLaunchService, "Moodle mobile webservice shortname")
	mobileLaunchLoginCmd.Flags().StringVar(&mobileLaunchPassport, "passport", "", "Launch passport; autogenerated when omitted")
	mobileLaunchLoginCmd.Flags().StringVar(&mobileLaunchScheme, "scheme", moodle.DefaultMobileLaunchScheme, "Custom URL scheme used for the returned deep link")
	mobileLaunchLoginCmd.Flags().StringVar(&mobileLaunchCallback, "callback", "", "Already returned deep-link callback URL to parse instead of opening Moodle")
	mobileLaunchLoginCmd.Flags().BoolVar(&mobileLaunchHeaded, "headed", false, "Show the browser while completing the launch flow")
	mobileLaunchLoginCmd.Flags().BoolVar(&mobileLaunchSkipCourses, "skip-courses", false, "Verify the token but skip listing courses")
	mobileLaunchLoginCmd.Flags().DurationVar(&mobileLaunchTimeout, "timeout", 60*time.Second, "Timeout for the browser launch flow")
	mobileQRCmd.AddCommand(mobileQRInspectCmd)
	mobileQRCmd.AddCommand(mobileQRLoginCmd)
	mobileCmd.AddCommand(mobileQRCmd)
	mobileCmd.AddCommand(mobileLaunchLoginCmd)
}

type mobileLaunchLoginOptions struct {
	SiteURL      string
	Service      string
	Passport     string
	URLScheme    string
	CallbackURL  string
	Headless     bool
	CheckCourses bool
	Timeout      time.Duration
}

func inspectMobileQRLink(raw string) (mobileQRInspectResult, error) {
	link, err := moodle.ParseMobileQRLink(raw)
	if err != nil {
		return mobileQRInspectResult{}, err
	}

	result := mobileQRInspectResult{
		Kind:                 "site-url",
		SiteURL:              link.SiteURL,
		UserID:               link.UserID,
		PublicConfigEndpoint: link.PublicConfigEndpoint(),
		SafetyNote:           "This command only inspects the link. It does not redeem the QR login key or create a Moodle session.",
	}

	if link.IsAutoLogin {
		result.Kind = "qr-auto-login"
		result.QRLoginKeyRedacted = moodle.RedactSecret(link.QRLoginKey)
		result.TokenEndpoint = link.MobileTokenEndpoint()
		result.TokenWSFunction = "tool_mobile_get_tokens_for_qr_login"
		result.SampleTokenRequest = buildQRTokenRequest(link)
	}

	return result, nil
}

func runMobileQRLogin(checkAPI bool) (mobileQRLoginResult, error) {
	client, err := ensureAuthenticatedClient()
	if err != nil {
		return mobileQRLoginResult{}, err
	}
	link, err := client.FetchMobileQRLink()
	if err != nil {
		return mobileQRLoginResult{}, err
	}
	token, err := client.ExchangeMobileQRToken(link)
	if err != nil {
		return mobileQRLoginResult{}, err
	}
	session := moodle.MobileSessionFromToken(token)
	session.SchoolID = client.School.ID
	if err := moodle.SaveMobileSession(opts.MobileSessionPath, session); err != nil {
		return mobileQRLoginResult{}, err
	}

	result := mobileQRLoginResult{
		Status:               "created",
		SiteURL:              token.SiteURL,
		UserID:               token.UserID,
		MobileSessionPath:    opts.MobileSessionPath,
		QRLoginKeyRedacted:   moodle.RedactSecret(token.QRLoginKey),
		TokenReceived:        token.Token != "",
		PrivateTokenReceived: token.PrivateToken != "",
		TokenRedacted:        moodle.RedactSecret(token.Token),
		PrivateTokenRedacted: moodle.RedactSecret(token.PrivateToken),
		SafetyNote:           "The real mobile token was saved locally and is not printed in full. Treat the mobile session file like a password.",
	}

	if !checkAPI {
		return result, nil
	}

	info, err := client.FetchMobileSiteInfo(token)
	if err != nil {
		return mobileQRLoginResult{}, err
	}
	result.SiteName = info.SiteName
	result.Username = info.UserName
	result.MobileFunctionChecked = "core_webservice_get_site_info"

	courses, err := client.FetchMobileUserCourses(token)
	if err != nil {
		return mobileQRLoginResult{}, err
	}
	result.CourseCount = len(courses)
	for i, course := range courses {
		if i >= 5 {
			break
		}
		result.SampleCourses = append(result.SampleCourses, mobileCourseSummary{
			ID:        course.ID,
			FullName:  course.FullName,
			ShortName: course.ShortName,
		})
	}

	return result, nil
}

func runMobileLaunchLogin(options mobileLaunchLoginOptions) (mobileLaunchLoginResult, error) {
	siteURL := strings.TrimRight(strings.TrimSpace(options.SiteURL), "/")
	if siteURL == "" {
		siteURL = moodle.GetDefaultSchool().MoodleURL
	}
	passport := strings.TrimSpace(options.Passport)
	var launchURL moodle.MobileLaunchURL
	var callback moodle.MobileLaunchToken
	var err error

	if strings.TrimSpace(options.CallbackURL) != "" {
		launchURL, err = moodle.BuildMobileLaunchURL(siteURL, options.Service, passport, options.URLScheme)
		if err != nil {
			return mobileLaunchLoginResult{}, err
		}
		callback, err = moodle.ParseMobileLaunchCallback(options.CallbackURL)
		if err != nil {
			return mobileLaunchLoginResult{}, err
		}
		if passport != "" && callback.SiteID != launchURL.SiteID {
			return mobileLaunchLoginResult{}, fmt.Errorf("mobile launch callback site id mismatch")
		}
	} else {
		if _, err := ensureAuthenticatedClient(); err != nil {
			return mobileLaunchLoginResult{}, err
		}
		session, err := moodle.LoadSession(opts.SessionPath)
		if err != nil {
			return mobileLaunchLoginResult{}, err
		}
		launched, err := moodle.LaunchMobileLoginWithSession(moodle.MobileLaunchOptions{
			SiteURL:   siteURL,
			Cookies:   session.Cookies,
			Service:   options.Service,
			Passport:  passport,
			URLScheme: options.URLScheme,
			Headless:  options.Headless,
			Timeout:   options.Timeout,
		})
		if err != nil {
			return mobileLaunchLoginResult{}, err
		}
		launchURL = launched.LaunchURL
		callback = launched.Callback
	}

	token := moodle.MobileTokenFromLaunch(launchURL.SiteURL, callback)
	info, err := moodle.FetchMobileSiteInfo(token)
	if err != nil {
		return mobileLaunchLoginResult{}, err
	}
	token.UserID = info.UserID
	session := moodle.MobileSessionFromToken(token)
	session.SchoolID = moodle.ActiveSchoolID
	if err := moodle.SaveMobileSession(opts.MobileSessionPath, session); err != nil {
		return mobileLaunchLoginResult{}, err
	}

	result := mobileLaunchLoginResult{
		Status:                "created",
		SiteURL:               token.SiteURL,
		UserID:                token.UserID,
		Username:              info.UserName,
		SiteName:              info.SiteName,
		LaunchURL:             launchURL.URL,
		URLScheme:             launchURL.URLScheme,
		PassportRedacted:      moodle.RedactSecret(launchURL.Passport),
		SiteIDMatched:         callback.SiteID == launchURL.SiteID,
		MobileSessionPath:     opts.MobileSessionPath,
		TokenReceived:         token.Token != "",
		PrivateTokenReceived:  token.PrivateToken != "",
		TokenRedacted:         moodle.RedactSecret(token.Token),
		PrivateTokenRedacted:  moodle.RedactSecret(token.PrivateToken),
		MobileFunctionChecked: "core_webservice_get_site_info",
		SafetyNote:            "The real mobile token was saved locally and is not printed in full. Treat the mobile session file like a password.",
	}

	if !options.CheckCourses {
		return result, nil
	}
	courses, err := moodle.FetchMobileUserCourses(token)
	if err != nil {
		return mobileLaunchLoginResult{}, err
	}
	result.CourseCount = len(courses)
	for i, course := range courses {
		if i >= 5 {
			break
		}
		result.SampleCourses = append(result.SampleCourses, mobileCourseSummary{
			ID:        course.ID,
			FullName:  course.FullName,
			ShortName: course.ShortName,
		})
	}
	return result, nil
}

func buildQRTokenRequest(link moodle.MobileQRLink) string {
	return fmt.Sprintf(
		"POST %s\n[{\"index\":0,\"methodname\":\"tool_mobile_get_tokens_for_qr_login\",\"args\":{\"qrloginkey\":\"%s\",\"userid\":\"%d\"}}]",
		link.MobileTokenEndpoint(),
		moodle.RedactSecret(link.QRLoginKey),
		link.UserID,
	)
}

func renderMobileLaunchLoginText(w io.Writer, result mobileLaunchLoginResult) error {
	if _, err := fmt.Fprintf(w, "status: %s\n", result.Status); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "site: %s\n", result.SiteURL); err != nil {
		return err
	}
	if result.SiteName != "" {
		if _, err := fmt.Fprintf(w, "site name: %s\n", result.SiteName); err != nil {
			return err
		}
	}
	if result.Username != "" {
		if _, err := fmt.Fprintf(w, "username: %s\n", result.Username); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintf(w, "user id: %d\n", result.UserID); err != nil {
		return err
	}
	if result.MobileSessionPath != "" {
		if _, err := fmt.Fprintf(w, "mobile session: %s\n", result.MobileSessionPath); err != nil {
			return err
		}
	}
	if result.LaunchURL != "" {
		if _, err := fmt.Fprintf(w, "launch URL: %s\n", result.LaunchURL); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintf(w, "url scheme: %s\n", result.URLScheme); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "passport: %s\n", result.PassportRedacted); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "site id matched: %t\n", result.SiteIDMatched); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "mobile token received: %t\n", result.TokenReceived); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "private token received: %t\n", result.PrivateTokenReceived); err != nil {
		return err
	}
	if result.CourseCount > 0 {
		if _, err := fmt.Fprintf(w, "courses visible through mobile API: %d\n", result.CourseCount); err != nil {
			return err
		}
	}
	for _, course := range result.SampleCourses {
		if _, err := fmt.Fprintf(w, "- %s (%s, %d)\n", course.FullName, course.ShortName, course.ID); err != nil {
			return err
		}
	}
	_, err := fmt.Fprintf(w, "note: %s\n", result.SafetyNote)
	return err
}

func renderMobileQRInspectText(w io.Writer, result mobileQRInspectResult) error {
	if _, err := fmt.Fprintf(w, "site: %s\n", result.SiteURL); err != nil {
		return err
	}
	if result.UserID != 0 {
		if _, err := fmt.Fprintf(w, "user id: %d\n", result.UserID); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintf(w, "type: %s\n", result.Kind); err != nil {
		return err
	}
	if result.QRLoginKeyRedacted != "" {
		if _, err := fmt.Fprintf(w, "qr login key: %s\n", result.QRLoginKeyRedacted); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintf(w, "public config check: %s with methodname=tool_mobile_get_public_config\n", result.PublicConfigEndpoint); err != nil {
		return err
	}
	if result.TokenEndpoint != "" {
		if _, err := fmt.Fprintf(w, "token exchange: %s with methodname=%s\n", result.TokenEndpoint, result.TokenWSFunction); err != nil {
			return err
		}
	}
	_, err := fmt.Fprintf(w, "note: %s\n", result.SafetyNote)
	return err
}

func renderMobileQRLoginText(w io.Writer, result mobileQRLoginResult) error {
	if _, err := fmt.Fprintf(w, "status: %s\n", result.Status); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "site: %s\n", result.SiteURL); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "user id: %d\n", result.UserID); err != nil {
		return err
	}
	if result.MobileSessionPath != "" {
		if _, err := fmt.Fprintf(w, "mobile session: %s\n", result.MobileSessionPath); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintf(w, "qr login key: %s\n", result.QRLoginKeyRedacted); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "mobile token received: %t\n", result.TokenReceived); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "private token received: %t\n", result.PrivateTokenReceived); err != nil {
		return err
	}
	if result.SiteName != "" {
		if _, err := fmt.Fprintf(w, "site name: %s\n", result.SiteName); err != nil {
			return err
		}
	}
	if result.Username != "" {
		if _, err := fmt.Fprintf(w, "username: %s\n", result.Username); err != nil {
			return err
		}
	}
	if result.CourseCount > 0 {
		if _, err := fmt.Fprintf(w, "courses visible through mobile API: %d\n", result.CourseCount); err != nil {
			return err
		}
	}
	for _, course := range result.SampleCourses {
		if _, err := fmt.Fprintf(w, "- %s (%s, %d)\n", course.FullName, course.ShortName, course.ID); err != nil {
			return err
		}
	}
	_, err := fmt.Fprintf(w, "note: %s\n", result.SafetyNote)
	return err
}
