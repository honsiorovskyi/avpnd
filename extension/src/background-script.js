// Firefox (Manifest V2) exposes `browser` + `browserAction`; Chrome (Manifest V3) `chrome` + `action`.
const api = typeof browser !== "undefined" && browser.runtime ? browser : chrome
const action = api.action || api.browserAction

const red = "#a00000"
const green = "#008000"
const grey = "#606060"
const yellow = "#b39200"
const white = "#ffffff"

// avpnd listens here (35001 is reserved by the AWS VPN Client for its SAML callback).
const AVPND = "http://127.0.0.1:35002"
// The AWS client's own SAML callback page; tabs landing here are closed.
const SAML_CALLBACK = "http://127.0.0.1:35001/"

async function getVPNStatus() {
    const r = await fetch(`${AVPND}/status`)
    if (!r.ok) throw new Error(`status: HTTP ${r.status}`)
    const j = await r.json()
    return j.status
}

function tag(text, bg, fg) {
    action.setBadgeText({
        text: text
    })
    action.setBadgeBackgroundColor({
        color: bg
    })

    if (fg && action.setBadgeTextColor) {
        action.setBadgeTextColor({
            color: fg
        })
    }
}

async function handleBrowserAction() {
    try {
        const s = await getVPNStatus()

        switch (s) {
            case "connected":
            case "connecting":
            case "disconnecting":
                await fetch(`${AVPND}/disconnect`)
                break
            case "disconnected":
                // The AWS VPN Client opens the SAML login in the browser itself.
                await fetch(`${AVPND}/connect`)
                break
            default:
                console.log("unknown status:", s)
                return
        }
        monitor()
    } catch (e) {
        console.log("error", e)
    }
}

// avpnd caches status for 1s, but every CLI call still lands in the AWS
// client's log, so poll fast only while something is changing.
const POLL_FAST = 1000
const POLL_IDLE = 3000
let timer

async function monitor() {
    clearTimeout(timer)
    const s = await refresh()
    timer = setTimeout(monitor, s === "connecting" || s === "disconnecting" ? POLL_FAST : POLL_IDLE)
}

// Updates the badge and returns the status (undefined on error).
async function refresh() {
    try {
        const s = await getVPNStatus()
        switch (s) {
            case "connected":
                tag("on", green)
                break
            case "disconnected":
                tag("off", grey)
                break
            case "connecting":
            case "disconnecting":
                tag("...", yellow, white)
                break
            default:
                tag("?", red)
                console.log("unknown status:", s)
        }
        return s
    } catch (e) {
        tag("err", red)
        console.log("error", e)
    }
}

function closeVPNTab(tabId, changeInfo, tab) {
    if (changeInfo.url === SAML_CALLBACK) {
        api.tabs.remove(tabId);
    }
}

api.tabs.onUpdated.addListener(closeVPNTab)
action.onClicked.addListener(handleBrowserAction)

// Chrome's MV3 service worker can be suspended, which stops the polling loop
// above; an alarm wakes it up again. (Not available in the Firefox build.)
if (api.alarms) {
    api.alarms.create("poll", { periodInMinutes: 0.5 })
    api.alarms.onAlarm.addListener(monitor)
}

monitor()
