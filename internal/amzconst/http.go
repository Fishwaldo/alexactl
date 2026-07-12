// SPDX-FileCopyrightText: 2026 Justin Hammond
// SPDX-License-Identifier: MIT

// Package amzconst holds constants modeled on aioamazondevices' const/
// subpackage. Pure data — no logic.
package amzconst

// HTTP error codes treated as warnings rather than failures.
const (
	HTTPError199 = 199
	HTTPError299 = 299
)

// Content types.
const (
	ContentTypeJSON   = "application/json"
	ContentTypeStream = "application/octet-stream"
)

// ArrayWrapper is the synthetic key under which we wrap an anonymous JSON
// array, so the response_to_json helper always returns a map.
const ArrayWrapper = "generatedArrayWrapper"

// Amazon iOS app identity values used to impersonate the Alexa iOS client.
const (
	AmazonAppBundleID           = "com.amazon.echo"
	AmazonAppID                 = "MAPiOSLib/6.0/ToHideRetailLink"
	AmazonAppName               = "AioAmazonDevices"
	AmazonAppVersion            = "2.2.663733.0"
	AmazonDeviceSoftwareVersion = "35602678"
	AmazonDeviceType            = "A2IVLV5VM2W81"
	AmazonClientOS              = "18.5"
)

// Site defaults.
const (
	DefaultSite = "https://www.amazon.com"
	FESite      = "https://www.amazon.co.jp"
)

// CSRF header / cookie names.
const (
	CSRFA2Z    = "anti-csrftoken-a2z"
	CSRFCookie = "csrf"
)

// User-Agent strings.
const (
	UserAgentAmazon  = "AmazonWebView/AmazonAlexa/" + AmazonAppVersion + "/iOS/" + AmazonClientOS + "/iPhone"
	UserAgentBrowser = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/141.0.0.0 Safari/537.36 Edg/141.0.0.0"
)

// Token refresh kinds.
const (
	RefreshAccessToken = "access_token"
	RefreshAuthCookies = "auth_cookies"
)

// URI paths on the alexa.amazon.<domain> host.
const (
	URIAutomations      = "/api/behaviors/v2/automations"
	URICapabilities     = "/v1/devices/@self/capabilities"
	URIDeviceVolumes    = "/api/devices/deviceType/dsn/audio/v1/allDeviceVolumes"
	URIDevices          = "/api/devices-v2/device"
	URIDnDStatusAll     = "/api/dnd/device-status-list"
	URIDnDStatusDevice  = "/api/dnd/status"
	URIHistoryData      = "/alexa-privacy/apd/rah/alexa-history-records-v2"
	URIHistoryFrontend  = "/alexa-privacy/apd/rvh"
	URIMediaControl     = "/api/np/command"
	URIMediaState       = "/api/np/list-media-sessions"
	URIMusicProviders   = "/api/behaviors/entities?skillId=amzn1.ask.1p.music"
	URISounds           = "/api/behaviors/entities?skillId=amzn1.ask.1p.sound"
	URINexusGraphQL     = "/nexus/v1/graphql"
	URISchedules        = "/api/notifications"
	URISignin           = "/ap/signin"
	URIBehaviorsPreview = "/api/behaviors/preview"
	URIReboot           = "/v1/alexa/device/reboot/%s/%s" // device_type, serial
)

// URIToDoBase is the v2 Alexa lists (to-do / shopping) API on the retail
// host (www.amazon.<tld>). Replaces the deprecated v1 alexashoppinglists
// endpoints. Separate from the alexa.amazon.<tld> service host used by
// every other URI constant above. Used by the ListsService.
const URIToDoBase = "/alexashoppinglists/api/v2/lists"

// HTTP/2 AVS gateway template (formatted with the home region).
const (
	HTTP2SiteTemplate      = "https://bob-dispatch-prod-{region}.amazon.com"
	HTTP2DirectivesVersion = "20160207"
)

// Alexa communications service (global host, not marketplace-scoped).
const (
	CommSite           = "https://alexa-comms-mobile-service.amazon.com"
	URICommPreferences = "/devicesTypes/%s/deviceId/%s/preferences" // device_type, serial
)
