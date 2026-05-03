package chatgptapp

import _ "embed"

const widgetURI = "ui://widget/moodle-browser-v7.html"
const resourceMimeType = "text/html;profile=mcp-app"
const widgetDomain = "https://moodle-services.vercel.app"

//go:embed widget_dist.html
var widgetHTML string
