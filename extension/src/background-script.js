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
    browser.browserAction.setBadgeText({
        text: text
    })
    browser.browserAction.setBadgeBackgroundColor({
        color: bg
    })

    if (fg && browser.browserAction.setBadgeTextColor) {
        browser.browserAction.setBadgeTextColor({
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
        browser.tabs.remove(tabId);
    }
}

// handle chromium
if (typeof globalThis.browser === "undefined" || Object.getPrototypeOf(globalThis.browser) !== Object.prototype) {
    browser = chrome
}

browser.tabs.onUpdated.addListener(closeVPNTab) 
browser.browserAction.onClicked.addListener(handleBrowserAction)
monitor()
