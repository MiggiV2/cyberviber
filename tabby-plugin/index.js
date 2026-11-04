// tabby-cyberviber: focuses the Tabby tab (or split pane) whose shell runs a given process.
// Listens on ~/.cyberviber/tabby/<renderer pid>.sock, one newline-delimited JSON request per connection:
//   {"cmd":"focus","pids":[90,80]}  → {"ok":true,"index":3,"title":"…"} | {"ok":false}
//   {"cmd":"list"}                  → {"ok":true,"tabs":[{"index":1,"title":"…","pids":[80]}]}
// Plain JS on purpose: Tabby compiles Angular modules at runtime, so no build step is needed.
'use strict'
const { NgModule } = require('@angular/core')
const { AppService, HostWindowService, SplitTabComponent } = require('tabby-core')
const fs = require('fs')
const net = require('net')
const os = require('os')
const path = require('path')

const DIR = path.join(os.homedir(), '.cyberviber', 'tabby')

function leaves (tab) {
  return tab instanceof SplitTabComponent ? tab.getAllTabs() : [tab]
}

async function shellPIDs (tab) {
  const pty = tab.session && tab.session.pty // private in the typings, public at runtime
  const out = []
  for (const fn of ['getTruePID', 'getPID']) {
    try {
      if (pty && typeof pty[fn] === 'function') out.push(await pty[fn]())
    } catch (e) { /* tab without local shell */ }
  }
  return out
}

async function focus (app, hostWindow, pids) {
  const want = new Set(pids)
  for (const [i, top] of app.tabs.entries()) {
    for (const tab of leaves(top)) {
      if ((await shellPIDs(tab)).some(p => want.has(p))) {
        app.selectTab(top)
        if (top !== tab && typeof top.focus === 'function') top.focus(tab)
        hostWindow.bringToFront()
        return { ok: true, index: i + 1, title: tab.customTitle || tab.title }
      }
    }
  }
  return { ok: false }
}

async function list (app) {
  const tabs = []
  for (const [i, top] of app.tabs.entries()) {
    for (const tab of leaves(top)) {
      tabs.push({ index: i + 1, title: tab.customTitle || tab.title, pids: await shellPIDs(tab) })
    }
  }
  return { ok: true, tabs }
}

function serve (app, hostWindow) {
  fs.mkdirSync(DIR, { recursive: true, mode: 0o700 })
  const sock = path.join(DIR, `${process.pid}.sock`)
  try { fs.unlinkSync(sock) } catch (e) { /* not there */ }
  const server = net.createServer(conn => {
    let buf = ''
    conn.setEncoding('utf8')
    conn.on('error', () => {})
    conn.on('data', async chunk => {
      buf += chunk
      const nl = buf.indexOf('\n')
      if (nl < 0) return
      let res
      try {
        const req = JSON.parse(buf.slice(0, nl))
        if (req.cmd === 'focus') res = await focus(app, hostWindow, req.pids || [])
        else if (req.cmd === 'list') res = await list(app)
        else res = { ok: false, error: 'unknown command' }
      } catch (e) {
        res = { ok: false, error: String(e) }
      }
      conn.end(JSON.stringify(res) + '\n')
    })
  })
  server.on('error', e => console.error('[cyberviber]', e))
  server.listen(sock, () => {
    try { fs.chmodSync(sock, 0o600) } catch (e) { /* ignore */ }
    console.info('[cyberviber] listening on', sock)
  })
  window.addEventListener('beforeunload', () => {
    server.close()
    try { fs.unlinkSync(sock) } catch (e) { /* ignore */ }
  })
}

class CyberviberModule {
  constructor (app, hostWindow) {
    serve(app, hostWindow)
  }
}
CyberviberModule.ctorParameters = () => [{ type: AppService }, { type: HostWindowService }]

exports.default = NgModule({})(CyberviberModule)
