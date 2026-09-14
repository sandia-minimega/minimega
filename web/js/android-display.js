// Copyright 2025-2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

/* Android Display - Screenshot streaming over WebSocket */
"use strict";

var androidDisplayWs = null;
var androidDisplayImg = null;
var androidDisplayDragging = false;
var androidDisplayLastSend = 0;
var androidDeviceWidth = 1080;
var androidDeviceHeight = 2424;
var androidConnected = false;

function getVmNameFromPath() {
    var parts = window.location.pathname.split('/').filter(Boolean);
    var vmIndex = parts.indexOf('vm');
    if (vmIndex === -1 || vmIndex + 1 >= parts.length) return null;
    return decodeURIComponent(parts[vmIndex + 1]);
}

function getAndroidApiBase() {
    var parts = window.location.pathname.split('/').filter(Boolean);
    var vmIndex = parts.indexOf('vm');
    if (vmIndex === -1 || vmIndex + 1 >= parts.length) return null;
    return '/' + parts.slice(0, vmIndex + 2).join('/') + '/android';
}

function getDisplayWsUrl() {
    var base = getAndroidApiBase();
    if (!base) return null;
    var proto = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    return proto + '//' + window.location.host + base + '/display/ws';
}

function setStatus(id, text, color) {
    var el = document.getElementById(id);
    if (!el) return;
    el.textContent = text;
    if (color) el.style.color = color;
}

function scaleToDevice(event, img) {
    var rect = img.getBoundingClientRect();
    var xp = event.clientX - rect.left;
    var yp = event.clientY - rect.top;
    var x = Math.round(xp / rect.width * androidDeviceWidth);
    var y = Math.round(yp / rect.height * androidDeviceHeight);
    if (x < 0 || x > androidDeviceWidth || y < 0 || y > androidDeviceHeight) return null;
    return {x: x, y: y};
}

function sendInput(msg) {
    if (androidDisplayWs && androidDisplayWs.readyState === WebSocket.OPEN) {
        androidDisplayWs.send(JSON.stringify(msg));
    }
}

function onDisplayPointerDown(event) {
    var surface = document.getElementById('android-input-surface');
    if (surface) surface.focus();

    var img = event.currentTarget;
    var pt = scaleToDevice(event, img);
    if (!pt) return;
    androidDisplayDragging = true;
    img.setPointerCapture(event.pointerId);
    sendInput({mouse: {x: pt.x, y: pt.y, buttons: event.buttons || 1}});
    event.preventDefault();
}

function onDisplayPointerMove(event) {
    if (!androidDisplayDragging) return;
    var now = Date.now();
    if (now - androidDisplayLastSend < 16) return;
    androidDisplayLastSend = now;

    var img = event.currentTarget;
    var pt = scaleToDevice(event, img);
    if (!pt) return;
    sendInput({mouse: {x: pt.x, y: pt.y, buttons: event.buttons}});
    event.preventDefault();
}

function onDisplayPointerUp(event) {
    if (!androidDisplayDragging) return;
    androidDisplayDragging = false;
    var img = event.currentTarget;
    var pt = scaleToDevice(event, img);
    if (!pt) return;
    sendInput({mouse: {x: pt.x, y: pt.y, buttons: 0}});
    event.preventDefault();
}

var scrollGesture = {active: false, cx: 0, cy: 0, endTimer: null, animId: null, targetY: 0};

function scrollGestureEnd() {
    if (!scrollGesture.active) return;
    sendInput({touch: {touches: [{x: scrollGesture.cx, y: scrollGesture.cy, identifier: 9, pressure: 0}]}});
    scrollGesture.active = false;
    scrollGesture.animId = null;
}

function scrollGestureTick() {
    var dy = scrollGesture.targetY - scrollGesture.cy;
    if (Math.abs(dy) < 2) {
        scrollGesture.animId = null;
        return;
    }
    var step = Math.round(dy * 0.4);
    if (step === 0) step = dy > 0 ? 1 : -1;
    scrollGesture.cy += step;
    if (scrollGesture.cy < 0) scrollGesture.cy = 0;
    if (scrollGesture.cy > androidDeviceHeight) scrollGesture.cy = androidDeviceHeight;
    sendInput({touch: {touches: [{x: scrollGesture.cx, y: scrollGesture.cy, identifier: 9, pressure: 50}]}});
    scrollGesture.animId = requestAnimationFrame(scrollGestureTick);
}

function onDisplayWheel(event) {
    event.preventDefault();
    event.stopPropagation();

    var img = document.getElementById('android-display');
    if (!img) return;
    var pt = scaleToDevice(event, img);
    if (!pt) return;

    var scrollPx = event.deltaY > 0 ? -150 : 150;

    if (!scrollGesture.active) {
        scrollGesture.active = true;
        scrollGesture.cx = pt.x;
        scrollGesture.cy = pt.y;
        scrollGesture.targetY = pt.y;
        sendInput({touch: {touches: [{x: pt.x, y: pt.y, identifier: 9, pressure: 50}]}});
    }

    scrollGesture.targetY += scrollPx;
    if (scrollGesture.targetY < 0) scrollGesture.targetY = 0;
    if (scrollGesture.targetY > androidDeviceHeight) scrollGesture.targetY = androidDeviceHeight;

    if (!scrollGesture.animId) {
        scrollGesture.animId = requestAnimationFrame(scrollGestureTick);
    }

    if (scrollGesture.endTimer) clearTimeout(scrollGesture.endTimer);
    scrollGesture.endTimer = setTimeout(scrollGestureEnd, 300);
}

var androidKeyboardActive = false;
var androidLandscape = false;

function onDisplayKeyDown(event) {
    if (!androidKeyboardActive) return;
    event.preventDefault();
    event.stopPropagation();
    if (event.repeat) return;
    sendInput({key: {key: event.key, eventType: 'keydown'}});
}

function onDisplayKeyUp(event) {
    if (!androidKeyboardActive) return;
    event.preventDefault();
    event.stopPropagation();
    sendInput({key: {key: event.key, eventType: 'keyup'}});
}

function sendHardwareButton(keyName) {
    sendInput({key: {key: keyName, eventType: 'keypress'}});
}

function toggleLandscape() {
    var base = getAndroidApiBase();
    if (!base) return;

    var newState = !androidLandscape;
    fetch(base + '/api/v1/emulator/rotation', {
        method: 'POST',
        headers: {'Content-Type': 'application/json'},
        body: JSON.stringify({landscape: newState})
    })
    .then(function(r) {
        if (!r.ok) throw new Error('HTTP ' + r.status);
        return r.json();
    })
    .then(function() {
        androidLandscape = newState;
        var btn = document.getElementById('hw-btn-landscape');
        if (!btn) return;
        var icon = btn.querySelector('.fa');
        if (newState) {
            btn.classList.add('landscape-active');
            btn.title = 'Switch to Portrait';
            if (icon) icon.classList.add('rotated');
        } else {
            btn.classList.remove('landscape-active');
            btn.title = 'Switch to Landscape';
            if (icon) icon.classList.remove('rotated');
        }
    })
    .catch(function() {});
}

function connectDisplay() {
    var url = getDisplayWsUrl();
    if (!url) {
        setStatus('display-status', 'Cannot determine WebSocket URL', '#d32f2f');
        return;
    }

    setStatus('display-status', 'Connecting...', '#666');
    var ws = new WebSocket(url);
    ws.binaryType = 'arraybuffer';

    var prevBlobUrl = null;

    ws.onopen = function() {
        androidConnected = true;
        androidDisplayWs = ws;
        setStatus('display-status', 'Connected', '#66bb6a');

        var img = document.getElementById('android-display');
        var placeholder = document.getElementById('display-placeholder');
        var buttons = document.getElementById('android-hardware-buttons');
        var gps = document.getElementById('android-gps-controls');
        var volNote = document.getElementById('vol-note');

        if (placeholder) placeholder.style.display = 'none';
        if (img) img.style.display = 'block';
        if (buttons) buttons.style.display = 'flex';
        if (gps) gps.style.display = 'block';
        if (volNote) volNote.style.display = 'block';
    };

    ws.onmessage = function(event) {
        if (!(event.data instanceof ArrayBuffer)) return;

        var blob = new Blob([event.data], {type: 'image/png'});
        var newUrl = URL.createObjectURL(blob);

        var img = document.getElementById('android-display');
        if (!img) return;

        img.onload = function() {
            if (prevBlobUrl) URL.revokeObjectURL(prevBlobUrl);
            prevBlobUrl = newUrl;

            if (img.naturalWidth > 0 && img.naturalHeight > 0) {
                androidDeviceWidth = img.naturalWidth;
                androidDeviceHeight = img.naturalHeight;
            }
        };
        img.src = newUrl;
    };

    ws.onclose = function() {
        androidConnected = false;
        androidDisplayWs = null;
        setStatus('display-status', 'Disconnected', '#d32f2f');

        setTimeout(function() {
            if (!androidConnected) connectDisplay();
        }, 3000);
    };

    ws.onerror = function() {
        setStatus('display-status', 'Connection error', '#d32f2f');
    };
}

function initGpsControls() {
    var presets = {
        'gps-preset-sf':     {lat: 37.7749,  lng: -122.4194},
        'gps-preset-nyc':    {lat: 40.7128,  lng: -74.0060},
        'gps-preset-london': {lat: 51.5074,  lng: -0.1278},
        'gps-preset-tokyo':  {lat: 35.6762,  lng: 139.6503}
    };

    Object.keys(presets).forEach(function(id) {
        var btn = document.getElementById(id);
        if (!btn) return;
        btn.addEventListener('click', function() {
            var p = presets[id];
            document.getElementById('gps-lat').value = p.lat;
            document.getElementById('gps-lng').value = p.lng;
            sendGps(p.lat, p.lng);
        });
    });

    var sendBtn = document.getElementById('gps-send');
    if (sendBtn) {
        sendBtn.addEventListener('click', function() {
            var lat = parseFloat(document.getElementById('gps-lat').value);
            var lng = parseFloat(document.getElementById('gps-lng').value);
            if (isNaN(lat) || isNaN(lng)) {
                setStatus('gps-status', 'Invalid coordinates', '#d32f2f');
                return;
            }
            sendGps(lat, lng);
        });
    }
}

function sendGps(lat, lng) {
    var base = getAndroidApiBase();
    if (!base) return;

    setStatus('gps-status', 'Sending...', '#666');

    fetch(base + '/api/v1/emulator/gps', {
        method: 'POST',
        headers: {'Content-Type': 'application/json'},
        body: JSON.stringify({latitude: lat, longitude: lng})
    })
    .then(function(r) {
        if (!r.ok) throw new Error('HTTP ' + r.status);
        return r.json();
    })
    .then(function() {
        setStatus('gps-status', 'GPS sent', '#2e7d32');
        setTimeout(function() { setStatus('gps-status', '', null); }, 3000);
    })
    .catch(function(err) {
        setStatus('gps-status', 'Failed: ' + err.message, '#d32f2f');
    });
}

function fetchStatus() {
    var base = getAndroidApiBase();
    if (!base) return;

    fetch(base + '/api/v1/emulator/status')
    .then(function(r) {
        if (!r.ok) throw new Error('HTTP ' + r.status);
        return r.json();
    })
    .then(function(data) {
        var w = parseInt((data.platformConfig || {})['hw.lcd.width'] || '1080', 10);
        var h = parseInt((data.platformConfig || {})['hw.lcd.height'] || '2424', 10);
        androidDeviceWidth = w;
        androidDeviceHeight = h;
    })
    .catch(function(err) {
        var errDiv = document.getElementById('status-error');
        if (errDiv) {
            errDiv.style.display = 'block';
            var msg = document.getElementById('error-message');
            if (msg) msg.textContent = err.message;
        }
    });
}

function initAndroidDisplay() {
    var vmName = getVmNameFromPath();
    var nameEl = document.getElementById('vm-name');
    if (nameEl) nameEl.textContent = vmName || 'Unknown';

    fetchStatus();

    var img = document.getElementById('android-display');
    if (img) {
        img.addEventListener('pointerdown', onDisplayPointerDown);
        img.addEventListener('pointermove', onDisplayPointerMove);
        img.addEventListener('pointerup', onDisplayPointerUp);
        img.addEventListener('pointercancel', onDisplayPointerUp);
        img.addEventListener('wheel', onDisplayWheel, {passive: false});
        img.addEventListener('contextmenu', function(e) { e.preventDefault(); });
    }

    var surface = document.getElementById('android-input-surface');
    if (surface) {
        surface.addEventListener('focus', function() { androidKeyboardActive = true; });
        surface.addEventListener('blur', function() { androidKeyboardActive = false; });
    }

    // Capture phase on document stops Firefox find-as-you-type before it fires
    document.addEventListener('keydown', onDisplayKeyDown, true);
    document.addEventListener('keyup', onDisplayKeyUp, true);

    var hwButtons = {
        'hw-btn-back': 'GoBack',
        'hw-btn-home': 'GoHome',
        'hw-btn-app-switch': 'AppSwitch',
        'hw-btn-power': 'Power',
        'hw-btn-vol-down': 'AudioVolumeDown',
        'hw-btn-vol-up': 'AudioVolumeUp'
    };
    Object.keys(hwButtons).forEach(function(id) {
        var btn = document.getElementById(id);
        if (btn) {
            btn.addEventListener('click', function() {
                sendHardwareButton(hwButtons[id]);
            });
        }
    });

    var landscapeBtn = document.getElementById('hw-btn-landscape');
    if (landscapeBtn) {
        landscapeBtn.addEventListener('click', toggleLandscape);
    }

    initGpsControls();
    connectDisplay();
}

if (typeof document !== 'undefined') {
    document.addEventListener('DOMContentLoaded', initAndroidDisplay);
}

if (typeof module !== 'undefined' && module.exports) {
    module.exports = {
        getVmNameFromPath: getVmNameFromPath,
        getAndroidApiBase: getAndroidApiBase,
        getDisplayWsUrl: getDisplayWsUrl,
        scaleToDevice: scaleToDevice,
        sendHardwareButton: sendHardwareButton
    };
}
