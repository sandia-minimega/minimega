/**
 * @name JSON-Editor Interactive Playground
 * @description The JSON-Editor Interactive Playground is a page where you can test various setups for the JSON Schema parser JSON-Editor
 * @version {{ VERSION }}
 * @author Peter Klein
 * @see https://github.com/pmk65/jedemov2/
 * @license MIT
 * @example see README.md for requirements, examples and usage info
 */

// value -> CSS/JavaScript mapping for external files
var mapping = {
  jsoneditor: {
    css: 'downloads/jsoneditor.min.css',
    js: 'downloads/jsoneditor.js'
  },
  theme: {
    spectre: {
      css: [
        'downloads/spectre.min.css',
        'downloads/spectre-exp.min.css'
      ]
    }
  },
  iconlib: {
    spectre: {
      css: 'downloads/spectre-icons.min.css'
    }
  },
  template: {
  },
  ext_lib: {
    lib_dompurify: {
      js: 'downloads/purify.min.js'
    }
  }
}

var extendObj = function (destination) {
  var source, i, property
  for (i = 1; i < arguments.length; i++) {
    source = arguments[i]
    for (property in source) {
      if (!source.hasOwnProperty(property)) continue
      if (source[property] && typeof source[property] === 'object' && source[property] !== null) {
        if (!destination.hasOwnProperty(property)) destination[property] = {}
        extendObj(destination[property], source[property])
      } else {
        destination[property] = source[property]
      }
    }
  }
  return destination
}

if (window.mappingOverride && typeof window.mappingOverride === 'object' && !Array.isArray(window.mappingOverride)) {
  extendObj(mapping, window.mappingOverride)
}

// Used to validate json on file drag/drop
const ajv = new Ajv({allErrors: true});

// Create a success notification instance
const successNotification = window.createNotification({
    theme: 'success',
    showDuration: 5000,
    displayCloseButton: true
});
// Create a fail notification instance
const errorNotification = window.createNotification({
    theme: 'error',
    showDuration: 5000,
    displayCloseButton: true
});

// Theme to use in ACE editor instances
var aceTheme = 'ace/theme/monokai'

// ACE Editor Beautify extension
var aceBeautify = window.ace.require('ace/ext/beautify')

// ACE Range addon
var aceRange = window.ace.require('ace/range').Range

// ACE Editor placeholders
var jeEditSchema = document.querySelector('#schema')
var jeEditStartval = document.querySelector('#startval')
var jeEditCode = document.querySelector('#editor')
var jeEditCSS = document.querySelector('#csseditor')
var jeOutput = document.querySelector('#output') // Form output
var jeValidate = document.querySelector('#validate') // Validation output

// ACE Editor instances
var aceCodeEditor
var aceStyleEditor
var aceSchemaEditor
var aceStartvalEditor
var aceOutputEditor
var aceValidateEditor

// Error Modal box
var jeModal = document.querySelector('.modal')
var jeModalContent = jeModal.querySelector('p')
var jeModalClose = jeModal.querySelector('.close-button')

// Iframe
var jeIframeEl = document.querySelector('iframe')
var jeIframe = jeIframeEl.contentWindow || (jeIframeEl.contentDocument.document || jeIframeEl.contentDocument)

// Options Checkboxes (Wrapper, not single checkboxes)
var jeCfg = document.querySelector('#json-editor-config') // Config wrapper
var jeExtlib = document.querySelector('#ext_lib')

// Example description placeholder
var jeExampleDesc = document.querySelector('#example-description')

// Slide-in config panel
var jeLeftPanel = document.querySelector('#slideleft-panel')
var jeRightPanel = document.querySelector('#slideright-panel')

// Buttons
var jeShowConfig = document.querySelector('#show-config') // Show config panel
var jeShowLoadExample = document.querySelector('#show-load-examples') // Show load examples panel
var jeSchemaLoad = document.querySelector('#external-schema') // Load schema
var jeExec = document.querySelector('#execute-code') // Create form from Schema
var jeDirectLink = document.querySelector('#direct_link') // Create direct link url
var jeUrlReset = document.querySelector('#direct_link_reset') // Clear query params from url
var jeTabs = document.querySelector('nav.tabs') // Tabs (Wrapper, not single buttons)
var jeDownloadExample = document.querySelector('#download_example')
var jeUploadExample = document.querySelector('#upload_example')
var jeFileSave = document.querySelector('#file-save')
var jeFileImport = document.querySelector('#file-import')
var jeFilesUsed = document.querySelector('#files-used')

var jeFileUpload = document.querySelector('input[type="file"]')
var jeDropZone = document.querySelector('#dropzone') // Drag'n'Drop upload zone

var jeValidationStatus = document.querySelector('#validationstatus')


// Split panels
var jeSplitCfg = {
  sizes: [75, 25],
  minSize: [200, 200],
  onDragEnd: function () {
    aceCodeEditor.resize()
    aceStyleEditor.resize()
    aceSchemaEditor.resize()
    aceStartvalEditor.resize()
    aceOutputEditor.resize()
    aceValidateEditor.resize()
  },
  gutter: function () {
    var gutter = document.createElement('div')
    gutter.className = 'split-gutter'
    gutter.style.height = '560px'
    return gutter
  },
  gutterSize: 4
}
var jeSplitPanels = [
  ['#split-panel1', '#split-panel2'],
  ['#split-panel3', '#split-panel4'],
  ['#split-panel5', '#split-panel6']
]

var jeBusyOverlay = document.querySelector('#busy-overlay') // Busy overlay

/* Helper functions */

// filter function for objects
Object.filter = function (obj, predicate) {
  var result = {}, key
  for (key in obj) {
    if (obj.hasOwnProperty(key) && predicate(obj[key])) result[key] = obj[key]
  }
  return result
}

// Tests if JSON schema is invalid. Returns errormsg if invalid
var isInvalidJson = function (code) {
  try { JSON.parse(code) } catch (e) { return 'Invalid Schema: ' + e.message.charAt(12).toUpperCase() + e.message.slice(13) }
  return false
}

// Click event handler - Toggle visibility state of modal box
var toggleModal = function () {
  jeModal.classList.toggle('show-modal')
}

// Click event handler - Close modal box if clicked outside
var closeModal = function (e) {
  if (e.target === jeModal && jeModal.classList.contains('show-modal')) toggleModal()
}

// Show JSON error in modal box
var showModalError = function () {
  var val = this.getValue()
  if (val.trim()) {
    var res = isInvalidJson(val)
    if (res) {
      jeModalContent.innerText = res
      toggleModal()
    }
  }
}

// Trigger event on element
var eventFire = function (el, etype) {
  if (el.fireEvent) el.fireEvent('on' + etype)
  else {
    el.dispatchEvent(new Event(etype, {
      'bubbles': true,
      'cancelable': false
    }))
  }
}

// Trigger mouse click event
var eventClickFire = function (el) {
  el.dispatchEvent(new MouseEvent('click', {
    'view': window,
    'bubbles': true,
    'cancelable': false
  }))
}

// Copy text to clipboard
var copyToClipboard = function (txt) {
  var ta = document.createElement('textarea')
  ta.value = txt
  ta.setAttribute('readonly', '')
  ta.style.position = 'fixed'
  ta.style.bottom = '0px'
  ta.style.left = '0px'
  document.body.appendChild(ta)
  ta.select()
  var res = true
  try { document.execCommand('copy') } catch (err) { res = false }
  document.body.removeChild(ta)
  return res
}

// Locked text
var getLockedText = function () {
  return ['// The following lines are mandatory and readonly. You can add custom code above and below.',
    'if (jseditor instanceof window.JSONEditor) jseditor.destroy();',
    'jseditor = new window.JSONEditor(document.querySelector("#json-editor-form"), jedata);'].join('\n')
}

// Get object of intersecting locked markers in Ace Editor
var getIntersectMarkers = function (newRange, keyString) {
  return Object.filter(aceCodeEditor.getSession().getMarkers(true), function (marker) {
    return marker.clazz == 'readonly-highlight' && marker.range.intersects(newRange) &&
             !(marker.range.isStart(newRange.end.row, newRange.end.column) && keyString !== 'delete') &&
             !(marker.range.isEnd(newRange.start.row, newRange.start.column) && keyString !== 'backspace')
  })
}

// Get object of locked markers inside coords in Ace Editor
var getInsideMarkers = function (row, column) {
  return Object.filter(aceCodeEditor.getSession().getMarkers(true), function (marker) {
    return marker.clazz == 'readonly-highlight' && marker.range.inside(row, column)
  })
}

// Get object of all locked markers in Ace Editor
var getAllMarkers = function () {
  return Object.filter(aceCodeEditor.getSession().getMarkers(true), function (marker) {
    return marker.clazz == 'readonly-highlight'
  })
}

// Create anchored range and marker in Ace Editor
var lockPosition = function (startRow, starColumn, endRow, endColumn) {
  var range = new aceRange(startRow - 1, starColumn, endRow, endColumn),
    session = aceCodeEditor.getSession()
  range.collapseRows()
  range.id = session.addMarker(range, 'readonly-highlight', 'line', true)
  range.start = session.doc.createAnchor(range.start)
  range.end = session.doc.createAnchor(range.end)
  range.end.$insertRight = true
  return range
}

var lockText = function () {
  var markers = getAllMarkers(), id
  // Remove existing locked markers
  for (id in markers) aceCodeEditor.getSession().removeMarker(id)

  var range = aceCodeEditor.find(getLockedText(), {caseSensitive: true})

  // Lock text
  if (range) {
    var endRow = aceCodeEditor.getSession().getLength() - 1,
      endColumn = aceCodeEditor.getSession().getLine(endRow).length

      // Add a linefeed if this is the last content in document
      // Otherwise last locked line will be in "text" and not "line" mode
    if (endRow == range.end.row && endColumn == range.end.column) {
      aceCodeEditor.getSession().insert({row: endRow + 1, column: 0}, '\n')
    }

    lockPosition(range.start.row + 1, range.start.column, range.end.row + 1, range.end.column)
    aceCodeEditor.getSession().getSelection().clearSelection()
  }
}

// Build list of external JavaScript and CSS files included in <head> of page
var listExternalFilesUsed = function () {
  var src = [], tags = jeIframe.document.querySelectorAll('head script:not([src=""]), head link[rel="stylesheet"]:not([href=""])')
  for (var tag in tags) {
    if (tags.hasOwnProperty(tag)) {
      src.push(tags[tag].src || tags[tag].href)
    }
  }
  var listToClipboardHandler = function (val) {
    jeModalContent.innerText = copyToClipboard(val) ? 'List copied to clipboard.' : 'Error: Copy to clipboard failed!'
  }
  jeModalContent.innerHTML = '<h5>List of external JavaScript and CSS files used in current example:</h5>' + src.sort().join('<br>') + '<br><br><div class="cbreq"><button id="cbreq-list">Copy list to Clipboard</button></div>'
  jeModalContent.querySelector('#cbreq-list').addEventListener('click', listToClipboardHandler.bind(null, src.sort().join('\n')), {once: true})
  toggleModal()
}

// Update values in schema from output JSON
var updateSchemaValues = function () {
  // this = aceOutputEditor
  var json
  try {
    json = JSON.parse(this.getValue())
  } catch (err) {
    jeModalContent.innerText = err
    toggleModal()
    return
  }
  jeIframe.updateSchemaValuesIframe(json)
}

// Handler for showing/hiding left panel
var setPanelHandler = function (panel) {
  var panelClose = panel.querySelector('.panel-close-button'),
    panelContainer = panel.querySelector('.panel-container'),
    panelHandler = function (e) {
      if (e.target === panel && !panel.classList.contains('panel-inactive') || !e.currentTarget.contains(panelContainer)) {
        if (panel.classList.contains('panel-active')) panel.classList.toggle('panel-inactive')
        else panel.classList.add('panel-active')
      }
      if (panel.classList.contains('panel-inactive') && panel.classList.contains('panel-left')) {
        // panel closing
        // Trigger generation of form
        eventFire(jeExec, 'click')
      }
    }
  panel.addEventListener('click', panelHandler, false)
  if (panelClose) panelClose.addEventListener('click', panelHandler, false)
  return panelHandler
}

// JSONP request
var loadJSONP = (function () {
  var unique = 0
  return function (url, callback, context) {
    var name = '_jsonp_' + unique++
    url += (url.match(/\?/) ? '&' : '?') + 'callback=' + name
    var script = document.createElement('script')
    script.src = url
    window[name] = function (data) {
      callback.call(context || window, data)
      document.querySelector('head').removeChild(script)
      script = null
      delete window[name]
    }
    document.querySelector('head').appendChild(script)
  }
})()

// Function to handle clicks on Tab buttons
var tabsHandler = function (e) {
  if (e.target && e.target.nodeName == 'BUTTON') {
    var buttons = this.querySelectorAll('button')
    for (var i = 0; i < buttons.length; i++) {
      buttons[i].classList.remove('active')
      document.querySelector(buttons[i].dataset.content).classList.remove('active')
    }
    e.target.classList.add('active')
    document.querySelector(e.target.dataset.content).classList.add('active')

    // Required for ACE editor to update the content after "display: hidden"
    aceSchemaEditor.resize()
    aceStartvalEditor.resize()
    aceCodeEditor.resize()
    aceStyleEditor.resize()
    aceOutputEditor.resize()
    aceValidateEditor.resize()
  }
}

// function to catch errors thrown inside iframe
window.iframeErrorCatcher = function (err) {
  jeModalContent.innerText = err
  toggleModal()
}

// catch form output and validation errors from inside iframe
window.iframeOutputCatcher = function (json, validation_errors, na) {
  var bu = document.querySelector('button[data-content="#content5"]')
  bu.style.display = na === 1 ? 'none' : 'initial'
  if (na !== 1) {
    aceOutputEditor.setValue(JSON.stringify(json, null, 2))
    aceOutputEditor.session.getSelection().clearSelection()
    aceOutputEditor.resize()
    // Show validation errors if there are any
    var val = validation_errors.length ? validation_errors : {'schema': 'valid'}
    jeValidationStatus.classList.remove('schema-invalid')
    if (validation_errors.length) jeValidationStatus.classList.add('schema-invalid')

    aceValidateEditor.setValue(JSON.stringify(val, null, 2))
    aceValidateEditor.session.getSelection().clearSelection()
    aceValidateEditor.resize()
    // Push changes to viewer
    viewerTopo = json;
    viewerRefresh();
  }
}

// Clear ACE Output and Validation editors
var clearOutput = function () {
  aceValidateEditor.setValue('')
  aceValidateEditor.session.getSelection().clearSelection()
  aceValidateEditor.resize()
  aceOutputEditor.setValue('')
  aceOutputEditor.session.getSelection().clearSelection()
  aceOutputEditor.resize()
}

// Convert URL GET parameters into object or return value if key is supplied
var getUrlParams = function (key) {
  var prmstr = window.location.search.substr(1), params = {}
  if (prmstr != null && prmstr !== '') {
    var prmarr = prmstr.split('&')
    for (var i = 0; i < prmarr.length; i++) {
      var tmparr = prmarr[i].split('=')
      if (typeof key !== 'undefined' && key == tmparr[0]) {
        params = tmparr[1]
        break
      }
      params[tmparr[0]] = tmparr[1]
    }
  }
  return params
}

// Convert object into query string
var toQueryString = function (obj) {
  var parts = []
  for (var i in obj) {
    if (obj.hasOwnProperty(i)) {
      parts.push(encodeURIComponent(i) + '=' + encodeURIComponent(obj[i]))
    }
  }
  return parts.join('&')
}

// Get options object from checkboxes and selectboxes
// if "data-json-editor-special" is set on tag, it will not be included
var getJsonEditorOptions = function () {
  var options = {},
    exclude = ':not([data-json-editor-special])',
    els = jeCfg.querySelectorAll('input[type="checkbox"]' + exclude + ',select' + exclude)
  Array.from(els).forEach(function (el) { // from() unsupported in IE
    if (el.tagName == 'SELECT') options[el.id] = el.value
    else options[el.value] = el.checked ? 1 : 0
    // else if (el.checked) options[el.value] = 1;//el.checked;
  })

  return options
}

// Create Direct Link URL with query parameters
var updateDirectLink = function (e) {
  var url = window.location.toString().replace(window.location.search, '')
  if (e.target == jeDirectLink) {
    url += '?schema=' + encodeURIComponent(window.LZString.compressToBase64(JSON.stringify(aceSchemaEditor.getValue().trim())))
    //        url += '&value=' +  window.LZString.compressToBase64(JSON.stringify(window.jsoneditor.getValue()));
    url += '&value=' + encodeURIComponent(window.LZString.compressToBase64(JSON.stringify(aceStartvalEditor.getValue().trim())))
    url += '&code=' + encodeURIComponent(window.LZString.compressToBase64(JSON.stringify(aceCodeEditor.getValue().trim())))
    url += '&style=' + encodeURIComponent(window.LZString.compressToBase64(JSON.stringify(aceStyleEditor.getValue().trim())))
    url += '&' + toQueryString(getJsonEditorOptions())

    var urlToClipboardHandler = function (val) {
      jeModalContent.innerText = copyToClipboard(val) ? 'URL copied to clipboard.' : 'Error: Copy to clipboard failed!'
    }

    if (window.location.protocol !== 'file:') {
      var encUrl = encodeURIComponent(url)
      if (encUrl.length < 5000) {
        loadJSONP('https://is.gd/create.php?format=json&url=' + encUrl, function (data) {
          // Clipboard actions not allowed here since it's a callback event and not an "User generated event"
          jeModalContent.innerHTML = '<div class="cbreq"><h3>Direct Link Generated</h3><button id="cbreq-url">Copy Url to Clipboard</button> <button id="cbreq-shorturl">Copy ShortUrl to Clipboard</button></div>'
          jeModalContent.querySelector('#cbreq-url').addEventListener('click', urlToClipboardHandler.bind(null, url), {once: true})
          jeModalContent.querySelector('#cbreq-shorturl').addEventListener('click', urlToClipboardHandler.bind(null, data.shorturl), {once: true})
          toggleModal()
        })

        if (window.history) {
          window.history.replaceState('', window.title, url)
        }
      } else {
        urlToClipboardHandler(url)
        toggleModal()
      }
    } else {
      urlToClipboardHandler(url)
      toggleModal()
    }
  } else {
    window.location.replace(url)
  }
  // window.location.href = url;
  // window.location.assign(url);
}

// Clear query parameters from URL
var resetUrl = function (e) {
  if (confirm('Clear URL query parameters?')) {
    if (window.localStorage) window.localStorage.setItem('jeplayground', '')
    updateDirectLink(e)
    aceCodeEditor.setValue('\n' + getLockedText() + '\n')
  }
}

// Set editors and config options based on JSON object
var loadConfig = function (response) {
  // editorConfig is statically set in this file, so 'response' should only be the schema
  var data = JSON.parse(response),
    schema = JSON.stringify(data, null, 2),
    startval = Object.keys(editorConfig.startval).length !== 0 ? JSON.stringify(editorConfig.startval, null, 2) : '',
    cfg = editorConfig.config,
    code = editorConfig.code,
    style = editorConfig.style,
    desc = editorConfig.desc

  // Clear include external library checkboxes
  Array.from(jeExtlib.querySelectorAll('input')).forEach(function (el) { // from() unsupported in IE
    el.checked = false
  })

  jeExampleDesc.innerHTML = ''
  clearOutput()

  // Add description of example to help page
  if (desc !== '' && desc != 'Add optional description here. (HTML format)') {
    jeModalContent.innerHTML = jeExampleDesc.innerHTML = '<h3>Info about "' + editorConfig.title + '" Example</h3>' + desc
    toggleModal()
  }

  // Update ACE Editor instances
  aceSchemaEditor.setValue(schema)
  aceSchemaEditor.session.getSelection().clearSelection()
  aceSchemaEditor.resize()

  aceStartvalEditor.setValue(startval)
  aceStartvalEditor.session.getSelection().clearSelection()
  aceStartvalEditor.resize()

  aceCodeEditor.setValue(code)
  aceCodeEditor.session.getSelection().clearSelection()
  aceCodeEditor.resize()
  lockText()

  aceStyleEditor.setValue(style)
  aceStyleEditor.session.getSelection().clearSelection()
  aceStyleEditor.resize()

  aceOutputEditor.resize()
  aceValidateEditor.resize()

  // Set config options
  for (var id in cfg) {
    if (cfg.hasOwnProperty(id)) {
      var el = jeCfg.querySelector('#' + id)
      if (el) {
        if (el.nodeName == 'INPUT' && el.type == 'checkbox') el.checked = cfg[id] || 0
        else if (el.nodeName == 'SELECT') el.value = cfg[id]
      }
    }
  }

  // Trigger generation of form
  eventFire(jeExec, 'click')
}

// Upload new topo JSON object
var updateFromFile = function (response) {
  clearOutput()
  // Update ACE Editor instances
  aceStartvalEditor.setValue(response)
  aceStartvalEditor.session.getSelection().clearSelection()
  aceStartvalEditor.resize()
  // Trigger generation of form
  eventFire(jeExec, 'click')
  // set viewer data
  viewerSchema = parseJson(aceSchemaEditor.getValue());
  viewerTopo = parseJson(aceStartvalEditor.getValue());
  viewerRefresh();
}

// Set editors and config options based on query parameters
var updateFromUrl = function () {
  var params = getUrlParams()
  clearOutput()
  if (params.code) {
    try {
      aceCodeEditor.setValue(JSON.parse(window.LZString.decompressFromBase64(decodeURIComponent(params.code))))
      aceCodeEditor.session.getSelection().clearSelection()
      lockText()
    } catch (err) {
      console.log('Error parsing Javascript data from URL parameter.', err)
    }
    delete params.code
  }
  if (params.style) {
    try {
      aceStyleEditor.setValue(JSON.parse(window.LZString.decompressFromBase64(decodeURIComponent(params.style))))
      aceStyleEditor.session.getSelection().clearSelection()
    } catch (err) {
      console.log('Error parsing CSS data from URL parameter.', err)
    }
    delete params.style
  }
  if (params.schema) {
    try {
      aceSchemaEditor.setValue(JSON.parse(window.LZString.decompressFromBase64(decodeURIComponent(params.schema))))
      aceSchemaEditor.session.getSelection().clearSelection()
    } catch (err) {
      console.log('Error parsing Schema data from URL parameter.', err)
    }
    delete params.schema
  }
  if (params.value) {
    try {
      aceStartvalEditor.setValue(JSON.parse(window.LZString.decompressFromBase64(decodeURIComponent(params.value))))
      aceStartvalEditor.session.getSelection().clearSelection()
    } catch (err) {
      console.log('Error parsing Startval data from URL parameter.', err)
    }
    delete params.value
  }

  aceSchemaEditor.resize()
  aceStartvalEditor.resize()
  aceCodeEditor.resize()
  aceStyleEditor.resize()
  aceOutputEditor.resize()
  aceValidateEditor.resize()

  for (var id in params) {
    if (params.hasOwnProperty(id)) {
      var el = document.querySelector('#' + id)
      if (el) {
        if (el.tagName == 'SELECT') el.value = params[id]
        else if (el.tagName == 'INPUT') el.checked = params[id] == '1'
      }
    }
  }

  // Trigger generation of form
  // eventFire(jeExec, 'click');
}

// Build codeblock to create JSON-Editor instance
var getCode = function (schema, startval) {
  var opt = 'schema:' + schema + (startval.trim() ? ', startval:' + startval : '')
  return 'var jseditor, jedata = {' + opt + '};'
  //      return 'if (jseditor) jseditor.destroy();var jseditor = new window.JSONEditor(document.querySelector("#json-editor-form"),{' + opt + '});';
}

// Fullscreen Drag'n'Drop upload handlers
var showDropZone = function () {
  jeDropZone.style.display = 'block'
}
var hideDropZone = function () {
  jeDropZone.style.display = 'none'
}

var handleDrop = function (e) {
  e.preventDefault()
  hideDropZone()
  var file = e.dataTransfer.files[0]
  if (file.type != 'application/json' || file.size === 0) {
    jeModalContent.innerText = 'Error: File uploaded is not a .JSON file'
    toggleModal()
    return
  }

  var reader = new FileReader()
  reader.onload = function (e) {
    var response = e.target.result
    var err = isInvalidJson(response)
    var schema = parseJson(aceSchemaEditor.getValue());
    var data = parseJson(response);
    var valid = ajv.validate(schema, data);
    if (err) {
      jeModalContent.innerText = err
      toggleModal()
      return
    } else if (!valid) {
      jeModalContent.innerText = 'Error: Schema validation errors with file uploaded: ' + ajv.errorsText();
      toggleModal();
      return
    }
    updateFromFile(response)
  }
  reader.readAsText(file)
}

var dragHandler = function (e) {
  switch ((this == window ? 'w_' : 'z_') + e.type) {
    case 'w_dragstart':
      this.disabled = true
      break
    case 'w_dragend':
      this.disabled = false
      break
    case 'w_dragenter':
      if (this.disabled !== true) showDropZone()
      break
    case 'z_dragenter':
    case 'z_dragover':
      e.dataTransfer.dropEffect = 'copy'
      break
    case 'z_dragleave':
      hideDropZone()
      break
    case 'z_drop':
      handleDrop(e)
      break
  }

  if (this.disabled !== true) e.preventDefault()
}

// Filter out duplicates from array
var uniqueArray = function (arr) {
  var seen = {}
  return arr.filter(function (item) {
    return seen.hasOwnProperty(item) ? false : seen[item] = true
  })
}

// Get CSS and JavaScript files listed in comments using special keywords
var getUserIncludes = function (code, regex) {
  var match = regex.exec(code), res = []
  while (match != null) {
    if (match[2].trim()) res.push(match[2])
    match = regex.exec(code)
  }
  return res
}

// Build list of external files to include in Iframe
var buildExtFiles = function (options, code) {
  var jsFiles = [], cssFiles = [], extFiles = '', map, styles = aceStyleEditor.getValue().trim()
  for (var i in options) {
    if (options.hasOwnProperty(i) && options[i] && (mapping.ext_lib[i] || mapping[i] && mapping[i][options[i]])) {
      map = mapping.ext_lib[i] || mapping[i][options[i]]
      if (map.js) jsFiles = jsFiles.concat(typeof map.js === 'string' ? [map.js] : map.js)
      if (map.css) cssFiles = cssFiles.concat(typeof map.css === 'string' ? [map.css] : map.css)
    }
  }

  // Include CSS and JavaScript files listed in comments using special keywords
  cssFiles = cssFiles.concat(getUserIncludes(code, /\s*\/\/\s*includeCSS\((['"])([^"']*)\1\)/g))
  jsFiles = jsFiles.concat(getUserIncludes(code, /\s*\/\/\s*includeJS\((['"])([^"']*)\1\)/g))

  cssFiles = uniqueArray(cssFiles)
  jsFiles = uniqueArray(jsFiles)

  if (cssFiles.length) extFiles += '<link rel="stylesheet" href="' + cssFiles.join('" /><link rel="stylesheet" href="') + '" />'
  if (jsFiles.length) extFiles += '<script src="' + jsFiles.join('"><\/script><script src="') + '"><\/script>'

  if (styles !== '') extFiles += '<style>' + styles + '</style>'

  return extFiles
}

// Build list of JSON-Editor options from config options
var buildEditorOptions = function (options) {
  var res = ''
  for (var i in options) {
    if (options.hasOwnProperty(i) && !/^lib_/.test(i)) {
      var val = typeof options[i] === 'string' ? '"' + options[i] + '"' : options[i]
      res += 'JSONEditor.defaults.options["' + i + '"] = ' + val + ';\n'
    }
  }
  return '<script>' + res + '</script>'
}

// Create page for Iframe
var createIframeContent = function (code) {
  var options = getJsonEditorOptions()
  return [
    '<!DOCTYPE HTML>',
    '<html lang="en">',
    '<head>',
    '<title>JSON-Editor Form</title>',
    '<meta http-equiv="content-type" content="text/html; charset=utf-8">',
    '<style>',
    'body {margin:0;padding:0;font: normal .9em/1.2 Arial;background-color:#333333 !important;}',
    '.inner-row {display: grid;background-color: #fff;position: relative;max-width: 1200px;left:50%;transform: translate(-50%,0);padding: 1rem 2rem;box-shadow: 2px 0 5px rgba(0,0,0,.2);margin:0 0 3rem 0;}',
    '</style>',
    '<link rel="stylesheet" href="' + mapping.jsoneditor.css + '" />',
    '<script src="' + mapping.jsoneditor.js + '"><\/script>',
    buildExtFiles(options, code),
    buildEditorOptions(options),
    '</head>',
    '<body>',
    '<div class="inner-row"><div id="json-editor-form"></div></div>',
    '<script>',
    // Update JSON-Editor values from top window
    'window.updateSchemaValuesIframe = function(json) {',
    'if (jseditor instanceof window.JSONEditor && !jseditor.destroyed) jseditor.setValue(json);',
    '};',
    'try{',
    code,
    'var updateOutput = function() {window.top.iframeOutputCatcher(jseditor.getValue(), jseditor.validate()); };',
    // Send form output and validation errors to top window
    'if (jseditor instanceof window.JSONEditor && !jseditor.destroyed) {',
    // 'jseditor.on("ready", catcher);',
    'jseditor.on("change",updateOutput);',
    '} else {',
    'window.top.iframeOutputCatcher(null, null, 1);',
    '}',
    '}',
    // Send iframe errors to top window
    'catch(err){',
    'if (window.top.iframeErrorCatcher) window.top.iframeErrorCatcher(err);else console.log(err);',
    '};',
    '<\/script>',
    '</body>',
    '</html>'].join('\n')
}

// Update form values in iframe (Currently uses the StartVal editor, but should be from a different source)
/*    var setValueIframe = function() {
    var val = aceStartvalEditor.getValue();
    if (val.trim() && jeIframe.jseditor) jeIframe.jseditor.setValue(JSON.parse(val));
  }; */

//  Parse JSON string and return JSON object. Empty object returned on error
var parseJson = function (str) {
  var res
  try { res = JSON.parse(str) } catch (e) { res = {} }
  return res
}

var uploadExampleHandler = function (e) {
  e.preventDefault()
  var files = e.target.files || e.dataTransfer.files
  if (files.length !== 0) {
    var file = files[0]

    var reader = new FileReader()
    reader.onload = function (e) {
      var response = e.target.result
      var err = isInvalidJson(response)
      var schema = parseJson(aceSchemaEditor.getValue());
      var data = parseJson(response);
      var valid = ajv.validate(schema, data);
      if (err) {
        jeModalContent.innerText = err
        toggleModal()
        return
      } else if (!valid) {
        jeModalContent.innerText = 'Error: Schema validation errors with file uploaded: ' + ajv.errorsText();
        toggleModal();
        return
      }
      updateFromFile(response)
    }
    reader.readAsText(file)
  }
}

// Load Schema, Start Value, JavaScript Styles and Config options from Browser LocalStorage
var loadFromLocalStorage = function () {
  if (window.localStorage) {
    var data = window.localStorage.getItem('jeplayground')
    if (data) loadConfig(data)
  }
}

// Save Schema, Start Value, JavaScript Styles and Config options in Browser LocalStorage
var saveToLocalStorage = function () {
  if (window.localStorage) {
    window.localStorage.setItem('jeplayground', JSON.stringify({
      'schema': parseJson(aceSchemaEditor.getValue()),
      'startval': parseJson(aceStartvalEditor.getValue()),
      'config': getJsonEditorOptions(),
      'code': aceCodeEditor.getValue().trim(),
      'style': aceStyleEditor.getValue().trim(),
      'desc': ''
    }, null, 2))
  }
}

// Save Schema, Start Value, JavaScript Styles and Config options in examples schema format
var downloadExampleHandler = function () {
  var title = prompt('Enter a name for the topology', 'mytopo')
  if (title === null) return
  filename = (title || 'mytopo').toLowerCase().replace(/[\s<>:"\\|*]/g, '-') + '.json',
  blob = new Blob([aceOutputEditor.getValue().trim()], {type: 'application/json;charset=utf-8'})
  if (window.navigator && window.navigator.msSaveOrOpenBlob) {
    window.navigator.msSaveOrOpenBlob(blob, filename)
  } else {
    var a = document.createElement('a')
    a.download = filename
    a.href = URL.createObjectURL(blob)
    a.dataset.downloadurl = ['text/plain', a.download, a.href].join(':')
    eventClickFire(a)
  }
}

// save topo file to server
var fileSaveHandler = function (e) {
  var overwrite = e.shiftKey;
  var append = overwrite ? "\n    * OVERWRITE ENABLED!" : "\n    * Overwrite disabled (cancel and hold SHIFT to overwrite)";
  var title = prompt('Enter a name for the topology:' + append, 'mytopo');
  if (title === null) return;
  filename = (title || 'mytopo').toLowerCase().replace(/[\s<>:"\\|*]/g, '-') + '.json';
  var json;
  try {
    json = JSON.parse(aceOutputEditor.getValue().trim());
  } catch (err) {
    jeModalContent.innerText = "ERROR: Output contains invalid JSON.";
    toggleModal();
  }
  var data = JSON.stringify({"filename": filename, "data": json, "overwrite": overwrite});
  var xmlhttp = new XMLHttpRequest();
  xmlhttp.onreadystatechange = function() {
    if (xmlhttp.readyState === 4 && xmlhttp.status === 200) {
      if (xmlhttp.responseText.includes('topology already exists')) {
        jeModalContent.innerText = 'Sorry, topology already exists.';
        toggleModal()
//        errorNotification({message: xmlhttp.responseText});
      } else if (xmlhttp.responseText.includes('Sorry')) {
        jeModalContent.innerText = xmlhttp.responseText;
        toggleModal()
//        errorNotification({message: xmlhttp.responseText});
      } else {
        jeModalContent.innerText = xmlhttp.responseText;
        toggleModal()
//        successNotification({message: xmlhttp.responseText});
      }
    }
  }
  xmlhttp.open("POST", "handler.php", true);
  xmlhttp.setRequestHeader("Content-type", "application/json");
  xmlhttp.send(data);
}

// import topo from server
var fileImportHandler = function () {
  var xmlhttp = new XMLHttpRequest();
  xmlhttp.onreadystatechange = function() {
    if (xmlhttp.readyState === 4 && xmlhttp.status === 200) {
      try {
        jeModalContent.innerText = '';
        var topos = JSON.parse(xmlhttp.responseText);
        topos.forEach(function (e) {
          var b = document.createElement("button");
          b.id = e
          b.onclick = function () {
              importFile(this.id);
          };
          b.innerHTML = e;
          var li = document.createElement("li");
          li.appendChild(b);
          jeModalContent.appendChild(li);
        });
      } catch (err) {
        if (xmlhttp.responseText.includes('no topologies were found')) {
          jeModalContent.innerText = 'Sorry, no topologies were found';
        } else {
          jeModalContent.innerText = xmlhttp.responseText;
        }
      }
      toggleModal();
    }
  }
  xmlhttp.open("GET", "handler.php", true);
  xmlhttp.send(null);
}

var importFile = function(id) {
  var xmlhttp = new XMLHttpRequest();
  xmlhttp.onreadystatechange = function() {
    if (xmlhttp.readyState === 4 && xmlhttp.status === 200) {
      updateFromFile(xmlhttp.responseText);
      jeModalContent.innerHTML = '';
      toggleModal();
    }
  }
  xmlhttp.open("GET", "handler.php?name=" + id, true);
  xmlhttp.send(null);
}

// Load external file
var loadFile = function (file, mimeType, callback) {
  if (window.fetch && window.File && window.FileReader && window.FileList && window.Blob) {
    fetch(file, {mode: 'no-cors'})
      .then(function (response) {
        if (!response.ok) {
          jeModalContent.innerText = "Sorry, there was an error loading the file: " + file;
          toggleModal()
        }
        return response.blob()
      }).then(function (blob) {
        var reader = new FileReader()
        reader.onload = function (e) {
          callback(e.target.result)
        }
        reader.readAsText(blob)
      })
      .catch(function () {
        callback('')
      })
  } else {
    // IOS Safari and other crappy browsers :D
    var xobj = new XMLHttpRequest()
    xobj.overrideMimeType(mimeType)
    xobj.open('GET', file, true)
    xobj.onreadystatechange = function () {
      if (xobj.readyState == 4) {
        if (xobj.status == '200') callback(xobj.responseText)
        else callback('')
      }
    }
    xobj.send(null)
  }
}

// Change event handler - Load selected JSON Schema into editor
var loadExampleFiles = function (e) {
  if (e.target.nodeName == 'BUTTON' && e.target.value) {
    eventClickFire(jeRightPanel) // Close panel
    loadFile('examples/' + e.target.value + '.json', 'application/json', loadConfig)
  }
}

// Get index of examples and populate selectbox with results
var getExamplesIndex = function () {
  var out = '<h2>JSON-Editor Example</h2>',
    examplesSort = function (x, y) { return (x.title > y.title) - (x.title < y.title) }
//  loadFile('json/editor-config.json', 'application/json', function (cfg) {
    loadFile('json/schema.json', 'application/json', function (cfg) {
    if (isInvalidJson(cfg)) {
      jeShowLoadExample.disabled = true
      jeShowLoadExample.style.cursor = 'not-allowed'
      jeShowLoadExample.title = 'No examples available due to invalid index file'
    } else {
      jeSchemaLoad.innerHTML = out
    }
  })
}

// Extend expand/collapse for details/summary tags, so that only one open is allowed
/*    var summaryOpenHandler = function(e) {
    if (e.target.nodeName == 'SUMMARY') {
      var details = this.querySelectorAll('details');
      for (var i=0;i<details.length;i++) {
        if (details[i] != e.target.parentNode) details[i].removeAttribute('open');
      }
    }
  }; */

// Handler for buttons in editor slidedown panel
var editorPanelButtonHandler = function (e) {
  // "this" is the ACE Editor instance
  if (e.target.dataset.action) {
    e.preventDefault()
    switch (e.target.dataset.action.toLowerCase()) {
      case 'search':
        this.execCommand('find')
        break
      case 'replace':
        this.execCommand('replace')
        break
      case 'beautify':
        aceBeautify.beautify(this.session)
        if (this.id === 'editor6') lockText() // id editor6 = aceCodeEditor
        break
      case 'wordwrap':
        this.setOption('wrap', this.getOption('wrap') == 'off')
        break
      case 'validate':
        // Trigger re-generation of form
        eventFire(jeExec, 'click')
        break
      case 'clear': {
        if (this.id === 'editor6') {
          this.setValue('\n' + getLockedText() + '\n')
          lockText()
        } else this.setValue('')
        this.session.getSelection().clearSelection()
        break
      }
    }
  }
}

// Set click event action for buttons in editor slidedown menus
var setEditorPanelButtons = function (edEl, ed) {
  try {
    var buttons = edEl.parentNode.querySelectorAll('.slidedown-menu button');
  } catch (e) {
    return
  }

  [].forEach.call(buttons, function (button) {
    button.addEventListener('click', editorPanelButtonHandler.bind(ed))
  })
}

// "OnReady" event for Iframe
var iframeOnReady = function (a, b) {
  return a.readyState === 'loading' ? a.addEventListener('DOMContentLoaded', b) : b()
}

// Callback for when iframe is ready
var iframeReady = function () {
  // console.log('Not busy');
  jeBusyOverlay.classList.remove('active')
  eventFire(document.querySelector('nav.tabs button:nth-of-type(1)'), 'click')
}

// Click event handler - Creates the form from the JSON schema
var generateForm = function (e) {
  e.preventDefault()

  // Trigger Tab switching when form is ready
  window.requestAnimationFrame(function () {
    // console.log('Busy');
    jeBusyOverlay.classList.add('active') // Doesn't seem to work
    iframeOnReady(jeIframe, iframeReady)
  })

  // var startVal = aceOutputEditor.getValue() || aceStartvalEditor.getValue();
  var startVal = aceStartvalEditor.getValue()

  // Get content of ACE editor schema, startval and JavaScript;
  var code = getCode(aceSchemaEditor.getValue(), startVal) + aceCodeEditor.getValue()

  /*
    // Alternative to document.write() which is deprecated
    var bData = new Blob([createIframeContent(code)], {type: 'text/html'});
    jeIframeEl.onload = function() { window.URL.revokeObjectURL(bData); };
    jeIframeEl.src = window.URL.createObjectURL(bData);
  */

  // Iframe needs to be regenerated to prevent windows object from being cached
  // https://stackoverflow.com/questions/42065729/clean-the-cache-of-iframe
  var newIframeEl = document.createElement('iframe'),
    parent = jeIframeEl.parentNode,
    nextSibling = jeIframeEl.nextSibling

  newIframeEl.className = jeIframeEl.className
  newIframeEl.id = jeIframeEl.id

  parent.removeChild(jeIframeEl)
  parent.insertBefore(newIframeEl, nextSibling)

  jeIframeEl = newIframeEl
  jeIframe = jeIframeEl.contentWindow || (jeIframeEl.contentDocument.document || jeIframeEl.contentDocument)

  // document.write() seems to be the only way if you want reliable path info from window.loctation. See test example: https://codepen.io/pmk/pen/wOwoyW
  jeIframe.document.open()
  jeIframe.document.write(createIframeContent(code))
  jeIframe.document.close()

  saveToLocalStorage()
}

var setVimMode = function () {
  aceCodeEditor.setKeyboardHandler('ace/keyboard/vim')
  aceStyleEditor.setKeyboardHandler('ace/keyboard/vim')
  aceSchemaEditor.setKeyboardHandler('ace/keyboard/vim')
  aceStartvalEditor.setKeyboardHandler('ace/keyboard/vim')
  aceOutputEditor.setKeyboardHandler('ace/keyboard/vim')
  aceValidateEditor.setKeyboardHandler('ace/keyboard/vim')
}

var unsetVimMode = function () {
  aceCodeEditor.setKeyboardHandler('')
  aceStyleEditor.setKeyboardHandler('')
  aceSchemaEditor.setKeyboardHandler('')
  aceStartvalEditor.setKeyboardHandler('')
  aceOutputEditor.setKeyboardHandler('')
  aceValidateEditor.setKeyboardHandler('')
}
// Create ACE Editor instance
var createEditor = function (el, options) {
  var replaceCmd = {
      name: 'replace',
      bindKey: {win: 'Ctrl-R', mac: 'Command-Option-F'},
      exec: function (editor) {
        window.ace.config.loadModule('ace/ext/searchbox', function (e) { e.Search(editor, true) })
      },
      readOnly: true
    }, ed = window.ace.edit(el)

  ed.setOptions({theme: aceTheme})
  ed.session.setOptions(extendObj({}, {
    tabSize: 2,
    useSoftTabs: true
  }, options))
  ed.renderer.setOptions({minLines: 40, maxLines: 40})
  // Change replace shortcut from Ctrl-H to Ctrl-R
  ed.commands.addCommand(replaceCmd)

  return ed
}

/* Setup */

// Add modal box events
jeModalClose.addEventListener('click', toggleModal)
window.addEventListener('click', closeModal)

// Setup ACE editor for editing Schema
aceSchemaEditor = createEditor(jeEditSchema, {mode: 'ace/mode/json'})

// Setup ACE editor for editing Schema start values
aceStartvalEditor = createEditor(jeEditStartval, {mode: 'ace/mode/json'})

// Setup ACE editor for editing output values
aceOutputEditor = createEditor(jeOutput, {mode: 'ace/mode/json'})

// Setup ACE editor for displayiong validation results
aceValidateEditor = createEditor(jeValidate, {mode: 'ace/mode/json'})

// Setup ACE editor for editing CSS
aceStyleEditor = createEditor(jeEditCSS, {
  mode: 'ace/mode/css',
  wrap: true,
  useWrapMode: true,
  indentedSoftWrap: true
})

// Setup ACE editor for editing JavaScript
aceCodeEditor = createEditor(jeEditCode, {
  mode: 'ace/mode/javascript',
  wrap: true,
  useWrapMode: true,
  indentedSoftWrap: true
})

setVimMode()

// Fix to make the marker positions update correctly
// https://github.com/ajaxorg/ace/issues/3687
aceCodeEditor.on('change', aceCodeEditor.$onChangeFrontMarker)

// Intercept keypress
aceCodeEditor.keyBinding.addKeyboardHandler({
  handleKeyboard: function (data, hash, keyString, keyCode, event) {
    if (hash === -1 || keyCode === -1 || (keyCode === 67 && hash === 1) || (keyCode <= 40 && keyCode >= 37)) return false
    var newRange = aceCodeEditor.getSelectionRange(), block = Object.keys(getIntersectMarkers(newRange, keyString)).length > 0
    if (keyString == 'return' && block) aceCodeEditor.selection.moveTo(newRange.end.row + 1, 0)
    return block ? {command: 'null', passEvent: false} : false
  }
})

// Prevent cutting text into the locked range using context menu
aceCodeEditor.on('cut', function (e) {
  if (Object.keys(getIntersectMarkers(e)).length) e.setEnd(e.start.row, e.start.column)
}, false)

// Prevent pasting text into the locked range using context menu
aceCodeEditor.on('paste', function (e) {
  if (Object.keys(getIntersectMarkers(aceCodeEditor.getSelectionRange())).length) e.text = ''
}, false)

// Override default ace editor instance $tryReplace function
// Prevent replace function from overwriting the locked range
aceCodeEditor.$tryReplace = (function (orgFunc) {
  return function (newRange) {
    return Object.keys(getIntersectMarkers(newRange)).length ? null : orgFunc.apply(this, arguments)
  }
})(aceCodeEditor.$tryReplace)

// Override default session moveText function
// Prevent moving text into the locked range
var session = aceCodeEditor.getSession()
session.moveText = (function (orgFunc) {
  return function (fromRange, toPosition) {
    // Is toPosition inside any markers?
    var block = Object.keys(getInsideMarkers(toPosition.row, toPosition.column)).length > 0
    // Does the content being moved contain any markers?
    if (!block) block = Object.keys(getIntersectMarkers(fromRange)).length > 0
    return block ? fromRange : orgFunc.apply(this, arguments)
  }
})(session.moveText)

lockText()

// Show error if JSON schema or startval is invalid
aceSchemaEditor.on('blur', showModalError.bind(aceSchemaEditor))
aceStartvalEditor.on('blur', showModalError.bind(aceStartvalEditor))

// Update form if output JSON is changed
aceOutputEditor.on('blur', updateSchemaValues.bind(aceOutputEditor))

// Set buttom event in editor slidedown panels.
setEditorPanelButtons(jeEditSchema, aceSchemaEditor)
setEditorPanelButtons(jeEditStartval, aceStartvalEditor)
setEditorPanelButtons(jeEditCode, aceCodeEditor)
setEditorPanelButtons(jeEditCSS, aceStyleEditor)
setEditorPanelButtons(jeOutput, aceOutputEditor)
setEditorPanelButtons(jeValidate, aceValidateEditor)

// Set events on tabs buttons
jeTabs.addEventListener('click', tabsHandler, false)

// Set handler for showing/hiding left panel
var panelLeftHandler = setPanelHandler(jeLeftPanel)
jeShowConfig.addEventListener('click', panelLeftHandler, false)

// Set handler for showing/hiding left panel
var panelRightHandler = setPanelHandler(jeRightPanel)
jeShowLoadExample.addEventListener('click', panelRightHandler, false)

// Set button event for loading external schemas
if (window.fetch && window.File && window.FileReader && window.FileList && window.Blob) {
  jeSchemaLoad.addEventListener('click', loadExampleFiles, false)
} else {
  jeShowLoadExample.disabled = true
  jeShowLoadExample.style.cursor = 'not-allowed'
  jeShowLoadExample.title = 'Not available locally due to CORS policy'
}

// Set button event for generating form
jeExec.addEventListener('click', generateForm, false)

jeFilesUsed.addEventListener('click', listExternalFilesUsed, false)
// Create the direct link URL
jeDirectLink.addEventListener('click', updateDirectLink, false)
jeUrlReset.addEventListener('click', resetUrl, false)

// Set button event for downloading as example
jeDownloadExample.addEventListener('click', downloadExampleHandler, false)
// Set button event for uploading example
jeUploadExample.addEventListener('click', eventClickFire.bind(null, jeFileUpload), false)
jeFileUpload.addEventListener('change', uploadExampleHandler, false)
jeFileSave.addEventListener('click', fileSaveHandler, false)
jeFileImport.addEventListener('click', fileImportHandler, false)

// Set event handler for details/summary tags
// jeCfg.addEventListener('click', summaryOpenHandler, false);

// Set Drag'n'Drop handlers
if (window.File && window.FileReader && window.FileList && window.Blob) {
  ['dragenter', 'dragstart', 'dragend'].forEach(function (ev) {
    window.addEventListener(ev, dragHandler, false)
  });
  ['dragenter', 'dragover', 'dragleave', 'drop'].forEach(function (ev) {
    jeDropZone.addEventListener(ev, dragHandler, false)
  })
}

// Set resizable split panels
window.Split(jeSplitPanels[0], jeSplitCfg)
window.Split(jeSplitPanels[1], jeSplitCfg)
window.Split(jeSplitPanels[2], jeSplitCfg)

// Populate examples selectbox
getExamplesIndex()

// static editor config
var editorConfig = {
  "title": "JSON-Editor_sceptre",
  "startval": {},
  "config": {
    "theme": "spectre",
    "iconlib": "spectre",
    "object_layout": "normal",
    "template": "default",
    "show_errors": "always",
    "required_by_default": 0,
    "no_additional_properties": 1,
    "display_required_only": 1,
    "remove_empty_properties": 0,
    "keep_oneof_values": 0,
    "ajax": 0,
    "ajaxCredentials": 0,
    "show_opt_in": 0,
    "disable_edit_json": 0,
    "disable_collapse": 0,
    "disable_properties": 0,
    "disable_array_add": 0,
    "disable_array_reorder": 0,
    "disable_array_delete": 0,
    "enable_array_copy": 1,
    "array_controls_top": 0,
    "disable_array_delete_all_rows": 1,
    "disable_array_delete_last_row": 0,
    "prompt_before_delete": 1,
    "lib_aceeditor": 0,
    "lib_autocomplete": 0,
    "lib_sceditor": 0,
    "lib_simplemde": 0,
    "lib_select2": 0,
    "lib_selectize": 0,
    "lib_choices": 0,
    "lib_flatpickr": 0,
    "lib_signaturepad": 0,
    "lib_mathjs": 0,
    "lib_cleavejs": 0,
    "lib_jodit": 0,
    "lib_jquery": 0,
    "lib_dompurify": 1
  },
  "code": "// The following lines are mandatory and readonly. You can add custom code above and below.\nif (jseditor instanceof window.JSONEditor) jseditor.destroy();\njseditor = new window.JSONEditor(document.querySelector(\"#json-editor-form\"), jedata);",
  "style": "",
  "desc": "Add optional description here. (HTML format)"
}

// load json schema
loadFile('json/schema.json', 'application/json', loadConfig)

// Update fields from query parameters
if (window.location.search) {
  updateFromUrl()
  // Trigger generation of form
  eventFire(jeExec, 'click')
} else {
  loadFromLocalStorage()
}

// hotkey to toggle VIM mode
// Ctrl+Alt+Shift+v
document.onkeyup = function(e) {
  if (e.ctrlKey && e.altKey && e.shiftKey && e.which == 86) {
    if (aceCodeEditor.getKeyboardHandler().$id == "ace/keyboard/vim") {
      unsetVimMode();
    } else {
      setVimMode();
    }
  }
};
