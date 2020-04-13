/**
 * Copyright (c) 2006-2012, JGraph Ltd
 */

/**
 * Constructs a new open dialog.
 */
var OpenDialog = function()
{
    var iframe = document.createElement('iframe');
    iframe.style.backgroundColor = 'transparent';
    iframe.allowTransparency = 'true';
    iframe.style.borderStyle = 'none';
    iframe.style.borderWidth = '0px';
    iframe.style.overflow = 'hidden';
    iframe.frameBorder = '0';
    
    // Adds padding as a workaround for box model in older IE versions
    var dx = (mxClient.IS_VML && (document.documentMode == null || document.documentMode < 8)) ? 20 : 0;
    
    iframe.setAttribute('width', (((Editor.useLocalStorage) ? 640 : 320) + dx) + 'px');
    iframe.setAttribute('height', (((Editor.useLocalStorage) ? 480 : 220) + dx) + 'px');
    iframe.setAttribute('src', OPEN_FORM);
    
    this.container = iframe;
};

/**
 * Constructs a new color dialog.
 */
var ColorDialog = function(editorUi, color, apply, cancelFn)
{
    this.editorUi = editorUi;
    
    var input = document.createElement('input');
    input.style.marginBottom = '10px';
    input.style.width = '216px';
    
    // Required for picker to render in IE
    if (mxClient.IS_IE)
    {
        input.style.marginTop = '10px';
        document.body.appendChild(input);
    }
    
    this.init = function()
    {
        if (!mxClient.IS_TOUCH)
        {
            input.focus();
        }
    };

    var picker = new jscolor.color(input);
    picker.pickerOnfocus = false;
    picker.showPicker();

    var div = document.createElement('div');
    jscolor.picker.box.style.position = 'relative';
    jscolor.picker.box.style.width = '230px';
    jscolor.picker.box.style.height = '100px';
    jscolor.picker.box.style.paddingBottom = '10px';
    div.appendChild(jscolor.picker.box);

    var center = document.createElement('center');
    
    function createRecentColorTable()
    {
        var table = addPresets((ColorDialog.recentColors.length == 0) ? ['FFFFFF'] :
                    ColorDialog.recentColors, 11, 'FFFFFF', true);
        table.style.marginBottom = '8px';
        
        return table;
    };
    
    function addPresets(presets, rowLength, defaultColor, addResetOption)
    {
        rowLength = (rowLength != null) ? rowLength : 12;
        var table = document.createElement('table');
        table.style.borderCollapse = 'collapse';
        table.setAttribute('cellspacing', '0');
        table.style.marginBottom = '20px';
        table.style.cellSpacing = '0px';
        var tbody = document.createElement('tbody');
        table.appendChild(tbody);

        var rows = presets.length / rowLength;
        
        for (var row = 0; row < rows; row++)
        {
            var tr = document.createElement('tr');
            
            for (var i = 0; i < rowLength; i++)
            {
                (function(clr)
                {
                    var td = document.createElement('td');
                    td.style.border = '1px solid black';
                    td.style.padding = '0px';
                    td.style.width = '16px';
                    td.style.height = '16px';
                    
                    if (clr == null)
                    {
                        clr = defaultColor;
                    }
                    
                    if (clr == 'none')
                    {
                        td.style.background = 'url(\'' + Dialog.prototype.noColorImage + '\')';
                    }
                    else
                    {
                        td.style.backgroundColor = '#' + clr;
                    }
                    
                    tr.appendChild(td);

                    if (clr != null)
                    {
                        td.style.cursor = 'pointer';
                        
                        mxEvent.addListener(td, 'click', function()
                        {
                            if (clr == 'none')
                            {
                                picker.fromString('ffffff');
                                input.value = 'none';
                            }
                            else
                            {
                                picker.fromString(clr);
                            }
                        });
                    }
                })(presets[row * rowLength + i]);
            }
            
            tbody.appendChild(tr);
        }
        
        if (addResetOption)
        {
            var td = document.createElement('td');
            td.setAttribute('title', mxResources.get('reset'));
            td.style.border = '1px solid black';
            td.style.padding = '0px';
            td.style.width = '16px';
            td.style.height = '16px';
            td.style.backgroundImage = 'url(\'' + Dialog.prototype.closeImage + '\')';
            td.style.backgroundPosition = 'center center';
            td.style.backgroundRepeat = 'no-repeat';
            td.style.cursor = 'pointer';
            
            tr.appendChild(td);

            mxEvent.addListener(td, 'click', function()
            {
                ColorDialog.resetRecentColors();
                table.parentNode.replaceChild(createRecentColorTable(), table);
            });
        }
        
        center.appendChild(table);
        
        return table;
    };

    div.appendChild(input);
    mxUtils.br(div);
    
    // Adds recent colors
    createRecentColorTable();
        
    // Adds presets
    var table = addPresets(this.presetColors);
    table.style.marginBottom = '8px';
    table = addPresets(this.defaultColors);
    table.style.marginBottom = '16px';

    div.appendChild(center);

    var buttons = document.createElement('div');
    buttons.style.textAlign = 'right';
    buttons.style.whiteSpace = 'nowrap';
    
    var cancelBtn = mxUtils.button(mxResources.get('cancel'), function()
    {
        editorUi.hideDialog();
        
        if (cancelFn != null)
        {
            cancelFn();
        }
    });
    cancelBtn.className = 'geBtn';

    if (editorUi.editor.cancelFirst)
    {
        buttons.appendChild(cancelBtn);
    }
    
    var applyFunction = (apply != null) ? apply : this.createApplyFunction();
    
    var applyBtn = mxUtils.button(mxResources.get('apply'), function()
    {
        var color = input.value;
        
        // Blocks any non-alphabetic chars in colors
        if (/(^#?[a-zA-Z0-9]*$)/.test(color))
        {
            if (color != 'none' && color.charAt(0) != '#')
            {
                color = '#' + color;
            }

            ColorDialog.addRecentColor((color != 'none') ? color.substring(1) : color, 12);
            applyFunction(color);
            editorUi.hideDialog();
        }
        else
        {
            editorUi.handleError({message: mxResources.get('invalidInput')});   
        }
    });
    applyBtn.className = 'geBtn gePrimaryBtn';
    buttons.appendChild(applyBtn);
    
    if (!editorUi.editor.cancelFirst)
    {
        buttons.appendChild(cancelBtn);
    }
    
    if (color != null)
    {
        if (color == 'none')
        {
            picker.fromString('ffffff');
            input.value = 'none';
        }
        else
        {
            picker.fromString(color);
        }
    }
    
    div.appendChild(buttons);
    this.picker = picker;
    this.colorInput = input;

    // LATER: Only fires if input if focused, should always
    // fire if this dialog is showing.
    mxEvent.addListener(div, 'keydown', function(e)
    {
        if (e.keyCode == 27)
        {
            editorUi.hideDialog();
            
            if (cancelFn != null)
            {
                cancelFn();
            }
            
            mxEvent.consume(e);
        }
    });
    
    this.container = div;
};

/**
 * Creates function to apply value
 */
ColorDialog.prototype.presetColors = ['E6D0DE', 'CDA2BE', 'B5739D', 'E1D5E7', 'C3ABD0', 'A680B8', 'D4E1F5', 'A9C4EB', '7EA6E0', 'D5E8D4', '9AC7BF', '67AB9F', 'D5E8D4', 'B9E0A5', '97D077', 'FFF2CC', 'FFE599', 'FFD966', 'FFF4C3', 'FFCE9F', 'FFB570', 'F8CECC', 'F19C99', 'EA6B66']; 

/**
 * Creates function to apply value
 */
ColorDialog.prototype.defaultColors = ['none', 'FFFFFF', 'E6E6E6', 'CCCCCC', 'B3B3B3', '999999', '808080', '666666', '4D4D4D', '333333', '1A1A1A', '000000', 'FFCCCC', 'FFE6CC', 'FFFFCC', 'E6FFCC', 'CCFFCC', 'CCFFE6', 'CCFFFF', 'CCE5FF', 'CCCCFF', 'E5CCFF', 'FFCCFF', 'FFCCE6',
        'FF9999', 'FFCC99', 'FFFF99', 'CCFF99', '99FF99', '99FFCC', '99FFFF', '99CCFF', '9999FF', 'CC99FF', 'FF99FF', 'FF99CC', 'FF6666', 'FFB366', 'FFFF66', 'B3FF66', '66FF66', '66FFB3', '66FFFF', '66B2FF', '6666FF', 'B266FF', 'FF66FF', 'FF66B3', 'FF3333', 'FF9933', 'FFFF33',
        '99FF33', '33FF33', '33FF99', '33FFFF', '3399FF', '3333FF', '9933FF', 'FF33FF', 'FF3399', 'FF0000', 'FF8000', 'FFFF00', '80FF00', '00FF00', '00FF80', '00FFFF', '007FFF', '0000FF', '7F00FF', 'FF00FF', 'FF0080', 'CC0000', 'CC6600', 'CCCC00', '66CC00', '00CC00', '00CC66',
        '00CCCC', '0066CC', '0000CC', '6600CC', 'CC00CC', 'CC0066', '990000', '994C00', '999900', '4D9900', '009900', '00994D', '009999', '004C99', '000099', '4C0099', '990099', '99004D', '660000', '663300', '666600', '336600', '006600', '006633', '006666', '003366', '000066',
        '330066', '660066', '660033', '330000', '331A00', '333300', '1A3300', '003300', '00331A', '003333', '001933', '000033', '190033', '330033', '33001A'];

/**
 * Creates function to apply value
 */
ColorDialog.prototype.createApplyFunction = function()
{
    return mxUtils.bind(this, function(color)
    {
        var graph = this.editorUi.editor.graph;
        
        graph.getModel().beginUpdate();
        try
        {
            graph.setCellStyles(this.currentColorKey, color);
            this.editorUi.fireEvent(new mxEventObject('styleChanged', 'keys', [this.currentColorKey],
                'values', [color], 'cells', graph.getSelectionCells()));
        }
        finally
        {
            graph.getModel().endUpdate();
        }
    });
};

/**
 * 
 */
ColorDialog.recentColors = [];

/**
 * Adds recent color for later use.
 */
ColorDialog.addRecentColor = function(color, max)
{
    if (color != null)
    {
        mxUtils.remove(color, ColorDialog.recentColors);
        ColorDialog.recentColors.splice(0, 0, color);
        
        if (ColorDialog.recentColors.length >= max)
        {
            ColorDialog.recentColors.pop();
        }
    }
};

/**
 * Adds recent color for later use.
 */
ColorDialog.resetRecentColors = function()
{
    ColorDialog.recentColors = [];
};

/**
 * Constructs a new about dialog.
 */
var AboutDialog = function(editorUi)
{
    var div = document.createElement('div');
    div.setAttribute('align', 'center');
    var h3 = document.createElement('h3');
    mxUtils.write(h3, mxResources.get('about') + ' GraphEditor');
    div.appendChild(h3);
    var img = document.createElement('img');
    img.style.border = '0px';
    img.setAttribute('width', '176');
    img.setAttribute('width', '151');
    img.setAttribute('src', IMAGE_PATH + '/logo.png');
    div.appendChild(img);
    mxUtils.br(div);
    mxUtils.write(div, 'Powered by mxGraph ' + mxClient.VERSION);
    mxUtils.br(div);
    var link = document.createElement('a');
    link.setAttribute('href', 'http://www.jgraph.com/');
    link.setAttribute('target', '_blank');
    mxUtils.write(link, 'www.jgraph.com');
    div.appendChild(link);
    mxUtils.br(div);
    mxUtils.br(div);
    var closeBtn = mxUtils.button(mxResources.get('close'), function()
    {
        editorUi.hideDialog();
    });
    closeBtn.className = 'geBtn gePrimaryBtn';
    div.appendChild(closeBtn);
    
    this.container = div;
};

/**
 * Constructs a new filename dialog.
 */
var FilenameDialog = function(editorUi, filename, buttonText, fn, label, validateFn, content, helpLink, closeOnBtn, cancelFn, hints, w)
{
    closeOnBtn = (closeOnBtn != null) ? closeOnBtn : true;
    var row, td;
    
    var table = document.createElement('table');
    var tbody = document.createElement('tbody');
    table.style.marginTop = '8px';
    
    row = document.createElement('tr');
    
    td = document.createElement('td');
    td.style.whiteSpace = 'nowrap';
    td.style.fontSize = '10pt';
    td.style.width = '120px';
    mxUtils.write(td, (label || mxResources.get('filename')) + ':');
    
    row.appendChild(td);
    
    var nameInput = document.createElement('input');
    nameInput.setAttribute('value', filename || '');
    nameInput.style.marginLeft = '4px';
    nameInput.style.width = (w != null) ? w + 'px' : '180px';
    
    var genericBtn = mxUtils.button(buttonText, function()
    {
        if (validateFn == null || validateFn(nameInput.value))
        {
            if (closeOnBtn)
            {
                editorUi.hideDialog();
            }
            
            fn(nameInput.value);
        }
    });
    genericBtn.className = 'geBtn gePrimaryBtn';
    
    this.init = function()
    {
        if (label == null && content != null)
        {
            return;
        }
        
        nameInput.focus();
        
        if (mxClient.IS_GC || mxClient.IS_FF || document.documentMode >= 5 || mxClient.IS_QUIRKS)
        {
            nameInput.select();
        }
        else
        {
            document.execCommand('selectAll', false, null);
        }
        
        // Installs drag and drop handler for links
        if (Graph.fileSupport)
        {
            // Setup the dnd listeners
            var dlg = table.parentNode;
            
            if (dlg != null)
            {
                var graph = editorUi.editor.graph;
                var dropElt = null;
                    
                mxEvent.addListener(dlg, 'dragleave', function(evt)
                {
                    if (dropElt != null)
                    {
                        dropElt.style.backgroundColor = '';
                        dropElt = null;
                    }
                    
                    evt.stopPropagation();
                    evt.preventDefault();
                });
                
                mxEvent.addListener(dlg, 'dragover', mxUtils.bind(this, function(evt)
                {
                    // IE 10 does not implement pointer-events so it can't have a drop highlight
                    if (dropElt == null && (!mxClient.IS_IE || document.documentMode > 10))
                    {
                        dropElt = nameInput;
                        dropElt.style.backgroundColor = '#ebf2f9';
                    }
                    
                    evt.stopPropagation();
                    evt.preventDefault();
                }));
                        
                mxEvent.addListener(dlg, 'drop', mxUtils.bind(this, function(evt)
                {
                    if (dropElt != null)
                    {
                        dropElt.style.backgroundColor = '';
                        dropElt = null;
                    }
    
                    if (mxUtils.indexOf(evt.dataTransfer.types, 'text/uri-list') >= 0)
                    {
                        nameInput.value = decodeURIComponent(evt.dataTransfer.getData('text/uri-list'));
                        genericBtn.click();
                    }
    
                    evt.stopPropagation();
                    evt.preventDefault();
                }));
            }
        }
    };

    td = document.createElement('td');
    td.style.whiteSpace = 'nowrap';
    td.appendChild(nameInput);
    row.appendChild(td);
    
    if (label != null || content == null)
    {
        tbody.appendChild(row);
        
        if (hints != null)
        {
            td.appendChild(FilenameDialog.createTypeHint(editorUi, nameInput, hints));
        }
    }
    
    if (content != null)
    {
        row = document.createElement('tr');
        td = document.createElement('td');
        td.colSpan = 2;
        td.appendChild(content);
        row.appendChild(td);
        tbody.appendChild(row);
    }
    
    row = document.createElement('tr');
    td = document.createElement('td');
    td.colSpan = 2;
    td.style.paddingTop = '20px';
    td.style.whiteSpace = 'nowrap';
    td.setAttribute('align', 'right');
    
    var cancelBtn = mxUtils.button(mxResources.get('cancel'), function()
    {
        editorUi.hideDialog();
        
        if (cancelFn != null)
        {
            cancelFn();
        }
    });
    cancelBtn.className = 'geBtn';
    
    if (editorUi.editor.cancelFirst)
    {
        td.appendChild(cancelBtn);
    }
    
    if (helpLink != null)
    {
        var helpBtn = mxUtils.button(mxResources.get('help'), function()
        {
            editorUi.editor.graph.openLink(helpLink);
        });
        
        helpBtn.className = 'geBtn';    
        td.appendChild(helpBtn);
    }

    mxEvent.addListener(nameInput, 'keypress', function(e)
    {
        if (e.keyCode == 13)
        {
            genericBtn.click();
        }
    });
    
    td.appendChild(genericBtn);
    
    if (!editorUi.editor.cancelFirst)
    {
        td.appendChild(cancelBtn);
    }

    row.appendChild(td);
    tbody.appendChild(row);
    table.appendChild(tbody);
    
    this.container = table;
};

/**
 * 
 */
FilenameDialog.filenameHelpLink = null;

/**
 * 
 */
FilenameDialog.createTypeHint = function(ui, nameInput, hints)
{
    var hint = document.createElement('img');
    hint.style.cssText = 'vertical-align:top;height:16px;width:16px;margin-left:4px;background-repeat:no-repeat;background-position:center bottom;cursor:pointer;';
    mxUtils.setOpacity(hint, 70);
    
    var nameChanged = function()
    {
        hint.setAttribute('src', Editor.helpImage);
        hint.setAttribute('title', mxResources.get('help'));
        
        for (var i = 0; i < hints.length; i++)
        {
            if (hints[i].ext.length > 0 &&
                nameInput.value.substring(nameInput.value.length -
                        hints[i].ext.length - 1) == '.' + hints[i].ext)
            {
                hint.setAttribute('src',  mxClient.imageBasePath + '/warning.png');
                hint.setAttribute('title', mxResources.get(hints[i].title));
                break;
            }
        }
    };
    
    mxEvent.addListener(nameInput, 'keyup', nameChanged);
    mxEvent.addListener(nameInput, 'change', nameChanged);
    mxEvent.addListener(hint, 'click', function(evt)
    {
        var title = hint.getAttribute('title');
        
        if (hint.getAttribute('src') == Editor.helpImage)
        {
            ui.editor.graph.openLink(FilenameDialog.filenameHelpLink);
        }
        else if (title != '')
        {
            ui.showError(null, title, mxResources.get('help'), function()
            {
                ui.editor.graph.openLink(FilenameDialog.filenameHelpLink);
            }, null, mxResources.get('ok'), null, null, null, 340, 90);
        }
        
        mxEvent.consume(evt);
    });
    
    nameChanged();
    
    return hint;
};

/**
 * Constructs a new textarea dialog.
 */
var TextareaDialog = function(editorUi, title, url, fn, cancelFn, cancelTitle, w, h,
    addButtons, noHide, noWrap, applyTitle, helpLink, customButtons)
{
    w = (w != null) ? w : 300;
    h = (h != null) ? h : 120;
    noHide = (noHide != null) ? noHide : false;
    var row, td;
    
    var table = document.createElement('table');
    var tbody = document.createElement('tbody');
    
    row = document.createElement('tr');
    
    td = document.createElement('td');
    td.style.fontSize = '10pt';
    td.style.width = '100px';
    mxUtils.write(td, title);
    
    row.appendChild(td);
    tbody.appendChild(row);

    row = document.createElement('tr');
    td = document.createElement('td');

    var nameInput = document.createElement('textarea');
    
    if (noWrap)
    {
        nameInput.setAttribute('wrap', 'off');
    }
    
    nameInput.setAttribute('spellcheck', 'false');
    nameInput.setAttribute('autocorrect', 'off');
    nameInput.setAttribute('autocomplete', 'off');
    nameInput.setAttribute('autocapitalize', 'off');
    
    mxUtils.write(nameInput, url || '');
    nameInput.style.resize = 'none';
    nameInput.style.width = w + 'px';
    nameInput.style.height = h + 'px';
    
    this.textarea = nameInput;

    this.init = function()
    {
        nameInput.focus();
        nameInput.scrollTop = 0;
    };

    td.appendChild(nameInput);
    row.appendChild(td);
    
    tbody.appendChild(row);

    row = document.createElement('tr');
    td = document.createElement('td');
    td.style.paddingTop = '14px';
    td.style.whiteSpace = 'nowrap';
    td.setAttribute('align', 'right');
    
    if (helpLink != null)
    {
        var helpBtn = mxUtils.button(mxResources.get('help'), function()
        {
            editorUi.editor.graph.openLink(helpLink);
        });
        helpBtn.className = 'geBtn';
        
        td.appendChild(helpBtn);
    }
    
    if (customButtons != null)
    {
        for (var i = 0; i < customButtons.length; i++)
        {
            (function(label, fn)
            {
                var customBtn = mxUtils.button(label, function(e)
                {
                    fn(e, nameInput);
                });
                customBtn.className = 'geBtn';
                
                td.appendChild(customBtn);
            })(customButtons[i][0], customButtons[i][1]);
        }
    }
    
    var cancelBtn = mxUtils.button(cancelTitle || mxResources.get('cancel'), function()
    {
        editorUi.hideDialog();
        
        if (cancelFn != null)
        {
            cancelFn();
        }
    });
    cancelBtn.className = 'geBtn';
    
    if (editorUi.editor.cancelFirst)
    {
        td.appendChild(cancelBtn);
    }
    
    if (addButtons != null)
    {
        addButtons(td, nameInput);
    }
    
    if (fn != null)
    {
        var genericBtn = mxUtils.button(applyTitle || mxResources.get('apply'), function()
        {
            if (!noHide)
            {
                editorUi.hideDialog();
            }
            
            fn(nameInput.value);
        });
        
        genericBtn.className = 'geBtn gePrimaryBtn';    
        td.appendChild(genericBtn);
    }
    
    if (!editorUi.editor.cancelFirst)
    {
        td.appendChild(cancelBtn);
    }

    row.appendChild(td);
    tbody.appendChild(row);
    table.appendChild(tbody);
    this.container = table;
};

var topoJsonToXmlDialog = function (json) {

    var div = document.createElement('div');
    div.style.textAlign = 'right';
    var textarea = document.createElement('textarea');
    textarea.setAttribute('wrap', 'off');
    textarea.setAttribute('spellcheck', 'false');
    textarea.setAttribute('autocorrect', 'off');
    textarea.setAttribute('autocomplete', 'off');
    textarea.setAttribute('autocapitalize', 'off');
    textarea.style.overflow = 'auto';
    textarea.style.resize = 'none';
    textarea.style.width = '600px';
    textarea.style.height = '360px';
    textarea.style.marginBottom = '16px';



};

/**
 * Constructs a new edit file dialog.
 */
var EditDiagramDialog = function(editorUi)
{
    var div = document.createElement('div');
    div.style.textAlign = 'right';
    var textarea = document.createElement('textarea');
    textarea.setAttribute('wrap', 'off');
    textarea.setAttribute('spellcheck', 'false');
    textarea.setAttribute('autocorrect', 'off');
    textarea.setAttribute('autocomplete', 'off');
    textarea.setAttribute('autocapitalize', 'off');
    textarea.style.overflow = 'auto';
    textarea.style.resize = 'none';
    textarea.style.width = '600px';
    textarea.style.height = '360px';
    textarea.style.marginBottom = '16px';
    
    textarea.value = mxUtils.getPrettyXml(editorUi.editor.getGraphXml());
    div.appendChild(textarea);
    
    this.init = function()
    {
        textarea.focus();
    };
    
    // Enables dropping files
    if (Graph.fileSupport)
    {
        function handleDrop(evt)
        {
            evt.stopPropagation();
            evt.preventDefault();
            
            if (evt.dataTransfer.files.length > 0)
            {
                var file = evt.dataTransfer.files[0];
                var reader = new FileReader();
                
                reader.onload = function(e)
                {
                    textarea.value = e.target.result;
                };
                
                reader.readAsText(file);
            }
            else
            {
                textarea.value = editorUi.extractGraphModelFromEvent(evt);
            }
        };
        
        function handleDragOver(evt)
        {
            evt.stopPropagation();
            evt.preventDefault();
        };

        // Setup the dnd listeners.
        textarea.addEventListener('dragover', handleDragOver, false);
        textarea.addEventListener('drop', handleDrop, false);
    }
    
    var cancelBtn = mxUtils.button(mxResources.get('cancel'), function()
    {
        editorUi.hideDialog();
    });
    cancelBtn.className = 'geBtn';
    
    if (editorUi.editor.cancelFirst)
    {
        div.appendChild(cancelBtn);
    }
    
    var select = document.createElement('select');
    select.style.width = '180px';
    select.className = 'geBtn';

    if (editorUi.editor.graph.isEnabled())
    {
        var replaceOption = document.createElement('option');
        replaceOption.setAttribute('value', 'replace');
        mxUtils.write(replaceOption, mxResources.get('replaceExistingDrawing'));
        select.appendChild(replaceOption);
    }

    var newOption = document.createElement('option');
    newOption.setAttribute('value', 'new');
    mxUtils.write(newOption, mxResources.get('openInNewWindow'));
    
    if (EditDiagramDialog.showNewWindowOption)
    {
        select.appendChild(newOption);
    }

    if (editorUi.editor.graph.isEnabled())
    {
        var importOption = document.createElement('option');
        importOption.setAttribute('value', 'import');
        mxUtils.write(importOption, mxResources.get('addToExistingDrawing'));
        select.appendChild(importOption);
    }

    div.appendChild(select);

    var okBtn = mxUtils.button(mxResources.get('ok'), function()
    {
        // Removes all illegal control characters before parsing
        var data = Graph.zapGremlins(mxUtils.trim(textarea.value));
        var error = null;
        
        if (select.value == 'new')
        {
            editorUi.hideDialog();
            editorUi.editor.editAsNew(data);
        }
        else if (select.value == 'replace')
        {
            editorUi.editor.graph.model.beginUpdate();
            try
            {
                editorUi.editor.setGraphXml(mxUtils.parseXml(data).documentElement);
                // LATER: Why is hideDialog between begin-/endUpdate faster?
                editorUi.hideDialog();
            }
            catch (e)
            {
                error = e;
            }
            finally
            {
                editorUi.editor.graph.model.endUpdate();                
            }
        }
        else if (select.value == 'import')
        {
            editorUi.editor.graph.model.beginUpdate();
            try
            {
                var doc = mxUtils.parseXml(data);
                var model = new mxGraphModel();
                var codec = new mxCodec(doc);
                codec.decode(doc.documentElement, model);
                
                var children = model.getChildren(model.getChildAt(model.getRoot(), 0));
                editorUi.editor.graph.setSelectionCells(editorUi.editor.graph.importCells(children));
                
                // LATER: Why is hideDialog between begin-/endUpdate faster?
                editorUi.hideDialog();
            }
            catch (e)
            {
                error = e;
            }
            finally
            {
                editorUi.editor.graph.model.endUpdate();                
            }
        }
            
        if (error != null)
        {
            mxUtils.alert(error.message);
        }
    });
    okBtn.className = 'geBtn gePrimaryBtn';
    div.appendChild(okBtn);
    
    if (!editorUi.editor.cancelFirst)
    {
        div.appendChild(cancelBtn);
    }

    this.container = div;
};

/**
 * 
 */
EditDiagramDialog.showNewWindowOption = true;

/**
 * Constructs a new export dialog.
 */
var ExportDialog = function(editorUi)
{
    var graph = editorUi.editor.graph;
    var bounds = graph.getGraphBounds();
    var scale = graph.view.scale;
    
    var width = Math.ceil(bounds.width / scale);
    var height = Math.ceil(bounds.height / scale);

    var row, td;
    
    var table = document.createElement('table');
    var tbody = document.createElement('tbody');
    table.setAttribute('cellpadding', (mxClient.IS_SF) ? '0' : '2');
    
    row = document.createElement('tr');
    
    td = document.createElement('td');
    td.style.fontSize = '10pt';
    td.style.width = '100px';
    mxUtils.write(td, mxResources.get('filename') + ':');
    
    row.appendChild(td);
    
    var nameInput = document.createElement('input');
    nameInput.setAttribute('value', editorUi.editor.getOrCreateFilename());
    nameInput.style.width = '180px';

    td = document.createElement('td');
    td.appendChild(nameInput);
    row.appendChild(td);
    
    tbody.appendChild(row);
        
    row = document.createElement('tr');
    
    td = document.createElement('td');
    td.style.fontSize = '10pt';
    mxUtils.write(td, mxResources.get('format') + ':');
    
    row.appendChild(td);
    
    var imageFormatSelect = document.createElement('select');
    imageFormatSelect.style.width = '180px';

    var pngOption = document.createElement('option');
    pngOption.setAttribute('value', 'png');
    mxUtils.write(pngOption, mxResources.get('formatPng'));
    imageFormatSelect.appendChild(pngOption);

    var gifOption = document.createElement('option');
    
    if (ExportDialog.showGifOption)
    {
        gifOption.setAttribute('value', 'gif');
        mxUtils.write(gifOption, mxResources.get('formatGif'));
        imageFormatSelect.appendChild(gifOption);
    }
    
    var jpgOption = document.createElement('option');
    jpgOption.setAttribute('value', 'jpg');
    mxUtils.write(jpgOption, mxResources.get('formatJpg'));
    imageFormatSelect.appendChild(jpgOption);

    var pdfOption = document.createElement('option');
    pdfOption.setAttribute('value', 'pdf');
    mxUtils.write(pdfOption, mxResources.get('formatPdf'));
    imageFormatSelect.appendChild(pdfOption);
    
    var svgOption = document.createElement('option');
    svgOption.setAttribute('value', 'svg');
    mxUtils.write(svgOption, mxResources.get('formatSvg'));
    imageFormatSelect.appendChild(svgOption);
    
    if (ExportDialog.showXmlOption)
    {
        var xmlOption = document.createElement('option');
        xmlOption.setAttribute('value', 'xml');
        mxUtils.write(xmlOption, mxResources.get('formatXml'));
        imageFormatSelect.appendChild(xmlOption);
    }

    td = document.createElement('td');
    td.appendChild(imageFormatSelect);
    row.appendChild(td);
    
    tbody.appendChild(row);
    
    row = document.createElement('tr');

    td = document.createElement('td');
    td.style.fontSize = '10pt';
    mxUtils.write(td, mxResources.get('zoom') + ' (%):');
    
    row.appendChild(td);
    
    var zoomInput = document.createElement('input');
    zoomInput.setAttribute('type', 'number');
    zoomInput.setAttribute('value', '100');
    zoomInput.style.width = '180px';

    td = document.createElement('td');
    td.appendChild(zoomInput);
    row.appendChild(td);

    tbody.appendChild(row);

    row = document.createElement('tr');

    td = document.createElement('td');
    td.style.fontSize = '10pt';
    mxUtils.write(td, mxResources.get('width') + ':');
    
    row.appendChild(td);
    
    var widthInput = document.createElement('input');
    widthInput.setAttribute('value', width);
    widthInput.style.width = '180px';

    td = document.createElement('td');
    td.appendChild(widthInput);
    row.appendChild(td);

    tbody.appendChild(row);
    
    row = document.createElement('tr');
    
    td = document.createElement('td');
    td.style.fontSize = '10pt';
    mxUtils.write(td, mxResources.get('height') + ':');
    
    row.appendChild(td);
    
    var heightInput = document.createElement('input');
    heightInput.setAttribute('value', height);
    heightInput.style.width = '180px';

    td = document.createElement('td');
    td.appendChild(heightInput);
    row.appendChild(td);

    tbody.appendChild(row);
    
    row = document.createElement('tr');
    
    td = document.createElement('td');
    td.style.fontSize = '10pt';
    mxUtils.write(td, mxResources.get('dpi') + ':');
    
    row.appendChild(td);
    
    var dpiSelect = document.createElement('select');
    dpiSelect.style.width = '180px';

    var dpi100Option = document.createElement('option');
    dpi100Option.setAttribute('value', '100');
    mxUtils.write(dpi100Option, '100dpi');
    dpiSelect.appendChild(dpi100Option);

    var dpi200Option = document.createElement('option');
    dpi200Option.setAttribute('value', '200');
    mxUtils.write(dpi200Option, '200dpi');
    dpiSelect.appendChild(dpi200Option);
    
    var dpi300Option = document.createElement('option');
    dpi300Option.setAttribute('value', '300');
    mxUtils.write(dpi300Option, '300dpi');
    dpiSelect.appendChild(dpi300Option);
    
    var dpi400Option = document.createElement('option');
    dpi400Option.setAttribute('value', '400');
    mxUtils.write(dpi400Option, '400dpi');
    dpiSelect.appendChild(dpi400Option);
    
    var dpiCustOption = document.createElement('option');
    dpiCustOption.setAttribute('value', 'custom');
    mxUtils.write(dpiCustOption, mxResources.get('custom'));
    dpiSelect.appendChild(dpiCustOption);

    var customDpi = document.createElement('input');
    customDpi.style.width = '180px';
    customDpi.style.display = 'none';
    customDpi.setAttribute('value', '100');
    customDpi.setAttribute('type', 'number');
    customDpi.setAttribute('min', '50');
    customDpi.setAttribute('step', '50');
    
    var zoomUserChanged = false;
    
    mxEvent.addListener(dpiSelect, 'change', function()
    {
        if (this.value == 'custom')
        {
            this.style.display = 'none';
            customDpi.style.display = '';
            customDpi.focus();
        }
        else
        {
            customDpi.value = this.value;
            
            if (!zoomUserChanged) 
            {
                zoomInput.value = this.value;
            }
        }
    });
    
    mxEvent.addListener(customDpi, 'change', function()
    {
        var dpi = parseInt(customDpi.value);
        
        if (isNaN(dpi) || dpi <= 0)
        {
            customDpi.style.backgroundColor = 'red';
        }
        else
        {
            customDpi.style.backgroundColor = '';

            if (!zoomUserChanged) 
            {
                zoomInput.value = dpi;
            }
        }   
    });
    
    td = document.createElement('td');
    td.appendChild(dpiSelect);
    td.appendChild(customDpi);
    row.appendChild(td);

    tbody.appendChild(row);
    
    row = document.createElement('tr');
    
    td = document.createElement('td');
    td.style.fontSize = '10pt';
    mxUtils.write(td, mxResources.get('background') + ':');
    
    row.appendChild(td);
    
    var transparentCheckbox = document.createElement('input');
    transparentCheckbox.setAttribute('type', 'checkbox');
    transparentCheckbox.checked = graph.background == null || graph.background == mxConstants.NONE;

    td = document.createElement('td');
    td.appendChild(transparentCheckbox);
    mxUtils.write(td, mxResources.get('transparent'));
    
    row.appendChild(td);
    
    tbody.appendChild(row);
    
    row = document.createElement('tr');

    td = document.createElement('td');
    td.style.fontSize = '10pt';
    mxUtils.write(td, mxResources.get('borderWidth') + ':');
    
    row.appendChild(td);
    
    var borderInput = document.createElement('input');
    borderInput.setAttribute('type', 'number');
    borderInput.setAttribute('value', ExportDialog.lastBorderValue);
    borderInput.style.width = '180px';

    td = document.createElement('td');
    td.appendChild(borderInput);
    row.appendChild(td);

    tbody.appendChild(row);
    table.appendChild(tbody);
    
    // Handles changes in the export format
    function formatChanged()
    {
        var name = nameInput.value;
        var dot = name.lastIndexOf('.');
        
        if (dot > 0)
        {
            nameInput.value = name.substring(0, dot + 1) + imageFormatSelect.value;
        }
        else
        {
            nameInput.value = name + '.' + imageFormatSelect.value;
        }
        
        if (imageFormatSelect.value === 'xml')
        {
            zoomInput.setAttribute('disabled', 'true');
            widthInput.setAttribute('disabled', 'true');
            heightInput.setAttribute('disabled', 'true');
            borderInput.setAttribute('disabled', 'true');
        }
        else
        {
            zoomInput.removeAttribute('disabled');
            widthInput.removeAttribute('disabled');
            heightInput.removeAttribute('disabled');
            borderInput.removeAttribute('disabled');
        }
        
        if (imageFormatSelect.value === 'png' || imageFormatSelect.value === 'svg')
        {
            transparentCheckbox.removeAttribute('disabled');
        }
        else
        {
            transparentCheckbox.setAttribute('disabled', 'disabled');
        }
        
        if (imageFormatSelect.value === 'png')
        {
            dpiSelect.removeAttribute('disabled');
            customDpi.removeAttribute('disabled');
        }
        else
        {
            dpiSelect.setAttribute('disabled', 'disabled');
            customDpi.setAttribute('disabled', 'disabled');
        }
    };
    
    mxEvent.addListener(imageFormatSelect, 'change', formatChanged);
    formatChanged();

    function checkValues()
    {
        if (widthInput.value * heightInput.value > MAX_AREA || widthInput.value <= 0)
        {
            widthInput.style.backgroundColor = 'red';
        }
        else
        {
            widthInput.style.backgroundColor = '';
        }
        
        if (widthInput.value * heightInput.value > MAX_AREA || heightInput.value <= 0)
        {
            heightInput.style.backgroundColor = 'red';
        }
        else
        {
            heightInput.style.backgroundColor = '';
        }
    };

    mxEvent.addListener(zoomInput, 'change', function()
    {
        zoomUserChanged = true;
        var s = Math.max(0, parseFloat(zoomInput.value) || 100) / 100;
        zoomInput.value = parseFloat((s * 100).toFixed(2));
        
        if (width > 0)
        {
            widthInput.value = Math.floor(width * s);
            heightInput.value = Math.floor(height * s);
        }
        else
        {
            zoomInput.value = '100';
            widthInput.value = width;
            heightInput.value = height;
        }
        
        checkValues();
    });

    mxEvent.addListener(widthInput, 'change', function()
    {
        var s = parseInt(widthInput.value) / width;
        
        if (s > 0)
        {
            zoomInput.value = parseFloat((s * 100).toFixed(2));
            heightInput.value = Math.floor(height * s);
        }
        else
        {
            zoomInput.value = '100';
            widthInput.value = width;
            heightInput.value = height;
        }
        
        checkValues();
    });

    mxEvent.addListener(heightInput, 'change', function()
    {
        var s = parseInt(heightInput.value) / height;
        
        if (s > 0)
        {
            zoomInput.value = parseFloat((s * 100).toFixed(2));
            widthInput.value = Math.floor(width * s);
        }
        else
        {
            zoomInput.value = '100';
            widthInput.value = width;
            heightInput.value = height;
        }
        
        checkValues();
    });
    
    row = document.createElement('tr');
    td = document.createElement('td');
    td.setAttribute('align', 'right');
    td.style.paddingTop = '22px';
    td.colSpan = 2;
    
    var saveBtn = mxUtils.button(mxResources.get('export'), mxUtils.bind(this, function()
    {
        if (parseInt(zoomInput.value) <= 0)
        {
            mxUtils.alert(mxResources.get('drawingEmpty'));
        }
        else
        {
            var name = nameInput.value;
            var format = imageFormatSelect.value;
            var s = Math.max(0, parseFloat(zoomInput.value) || 100) / 100;
            var b = Math.max(0, parseInt(borderInput.value));
            var bg = graph.background;
            var dpi = Math.max(1, parseInt(customDpi.value));
            
            if ((format == 'svg' || format == 'png') && transparentCheckbox.checked)
            {
                bg = null;
            }
            else if (bg == null || bg == mxConstants.NONE)
            {
                bg = '#ffffff';
            }
            
            ExportDialog.lastBorderValue = b;
            ExportDialog.exportFile(editorUi, name, format, bg, s, b, dpi);
        }
    }));
    saveBtn.className = 'geBtn gePrimaryBtn';
    
    var cancelBtn = mxUtils.button(mxResources.get('cancel'), function()
    {
        editorUi.hideDialog();
    });
    cancelBtn.className = 'geBtn';
    
    if (editorUi.editor.cancelFirst)
    {
        td.appendChild(cancelBtn);
        td.appendChild(saveBtn);
    }
    else
    {
        td.appendChild(saveBtn);
        td.appendChild(cancelBtn);
    }

    row.appendChild(td);
    tbody.appendChild(row);
    table.appendChild(tbody);
    this.container = table;
};

/**
 * Remembers last value for border.
 */
ExportDialog.lastBorderValue = 0;

/**
 * Global switches for the export dialog.
 */
ExportDialog.showGifOption = true;

/**
 * Global switches for the export dialog.
 */
ExportDialog.showXmlOption = true;

/**
 * Hook for getting the export format. Returns null for the default
 * intermediate XML export format or a function that returns the
 * parameter and value to be used in the request in the form
 * key=value, where value should be URL encoded.
 */
ExportDialog.exportFile = function(editorUi, name, format, bg, s, b, dpi)
{
    var graph = editorUi.editor.graph;
    
    if (format == 'xml')
    {
        ExportDialog.saveLocalFile(editorUi, mxUtils.getXml(editorUi.editor.getGraphXml()), name, format);
    }
    else if (format == 'svg')
    {
        ExportDialog.saveLocalFile(editorUi, mxUtils.getXml(graph.getSvg(bg, s, b)), name, format);
    }
    else
    {
        var bounds = graph.getGraphBounds();
        
        // New image export
        var xmlDoc = mxUtils.createXmlDocument();
        var root = xmlDoc.createElement('output');
        xmlDoc.appendChild(root);
        
        // Renders graph. Offset will be multiplied with state's scale when painting state.
        var xmlCanvas = new mxXmlCanvas2D(root);
        xmlCanvas.translate(Math.floor((b / s - bounds.x) / graph.view.scale),
            Math.floor((b / s - bounds.y) / graph.view.scale));
        xmlCanvas.scale(s / graph.view.scale);
        
        var imgExport = new mxImageExport()
        imgExport.drawState(graph.getView().getState(graph.model.root), xmlCanvas);
        
        // Puts request data together
        var param = 'xml=' + encodeURIComponent(mxUtils.getXml(root));
        var w = Math.ceil(bounds.width * s / graph.view.scale + 2 * b);
        var h = Math.ceil(bounds.height * s / graph.view.scale + 2 * b);
        
        // Requests image if request is valid
        if (param.length <= MAX_REQUEST_SIZE && w * h < MAX_AREA)
        {
            editorUi.hideDialog();
            var req = new mxXmlRequest(EXPORT_URL, 'format=' + format +
                '&filename=' + encodeURIComponent(name) +
                '&bg=' + ((bg != null) ? bg : 'none') +
                '&w=' + w + '&h=' + h + '&' + param +
                '&dpi=' + dpi);
            req.simulate(document, '_blank');
        }
        else
        {
            mxUtils.alert(mxResources.get('drawingTooLarge'));
        }
    }
};

/**
 * Hook for getting the export format. Returns null for the default
 * intermediate XML export format or a function that returns the
 * parameter and value to be used in the request in the form
 * key=value, where value should be URL encoded.
 */
ExportDialog.saveLocalFile = function(editorUi, data, filename, format)
{
    if (data.length < MAX_REQUEST_SIZE)
    {
        editorUi.hideDialog();
        var req = new mxXmlRequest(SAVE_URL, 'xml=' + encodeURIComponent(data) + '&filename=' +
            encodeURIComponent(filename) + '&format=' + format);
        req.simulate(document, '_blank');
    }
    else
    {
        mxUtils.alert(mxResources.get('drawingTooLarge'));
        mxUtils.popup(xml);
    }
};

/**
 * Constructs a new metadata dialog.
 */
var EditDataDialog = function(ui, cell)
{

    const schema = {
        "$id": "#/properties/nodes",
        "type": "object",
        "title": "Node",
        // "headerTemplate": "{{i1}} - {{self.general.hostname}}",
        "required": [
          "id",
          "type",
          "general",
          "hardware"
        ],
        "properties": {
          "id": {
            "$id": "#/properties/nodes/items/properties/id",
            "type": "string",
            "title": "Id",
            "readOnly": true,
            "minLength": 1,
            "default": "",
            "examples": [
              "101"
            ],
            "pattern": "^(.*)$"
          },
          "type": {
            "$id": "#/properties/nodes/items/properties/type",
            "type": "string",
            "title": "Type",
            "enum": [
              "Firewall",
              "Printer",
              "Server",
              "Switch",
              "Router",
              "SCEPTRE",
              "VirtualMachine"
            ],
            "default": "VirtualMachine",
            "examples": [
              "Firewall",
              "Printer",
              "Server",
              "Switch",
              "Router",
              "SCEPTRE",
              "VirtualMachine"
            ],
            "pattern": "^(.*)$"
          },
          "general": {
            "$id": "#/properties/nodes/items/properties/general",
            "type": "object",
            "title": "General",
            "required": [
              "hostname"
            ],
            "properties": {
              "hostname": {
                "$id": "#/properties/nodes/items/properties/general/properties/hostname",
                "type": "string",
                "title": "Hostname",
                "minLength": 1,
                "default": "",
                "examples": [
                  "power-provider"
                ],
                "pattern": "^[\\w-]+$"
              },
              "description": {
                "$id": "#/properties/nodes/items/properties/general/properties/description",
                "type": "string",
                "title": "description",
                "default": "",
                "examples": [
                  "SCEPTRE power solver"
                ],
                "pattern": "^(.*)$"
              },
              "snapshot": {
                "$id": "#/properties/nodes/items/properties/general/properties/snapshot",
                "type": "boolean",
                "title": "snapshot",
                "default": false,
                "examples": [
                  false
                ]
              },
              "do_not_boot": {
                "$id": "#/properties/nodes/items/properties/general/properties/do_not_boot",
                "type": "boolean",
                "title": "do_not_boot",
                "default": false,
                "examples": [
                  false
                ]
              }
            }
          },
          "hardware": {
            "$id": "#/properties/nodes/items/properties/hardware",
            "type": "object",
            "title": "Hardware",
            "required": [
              "os_type",
              "drives"
            ],
            "properties": {
              "cpu": {
                "$id": "#/properties/nodes/items/properties/hardware/properties/cpu",
                "type": "string",
                "title": "cpu",
                "default": "Broadwell",
                "examples": [
                  "Broadwell",
                  "Haswell",
                  "core2duo",
                  "pentium3"
                ],
                "pattern": "^(.*)$"
              },
              "vcpus": {
                "$id": "#/properties/nodes/items/properties/hardware/properties/vcpus",
                "type": "string",
                "title": "vcpus",
                "default": "1",
                "examples": [
                  "4"
                ],
                "pattern": "^(.*)$"
              },
              "memory": {
                "$id": "#/properties/nodes/items/properties/hardware/properties/memory",
                "type": "string",
                "title": "memory",
                "default": "1024",
                "examples": [
                  "8192"
                ],
                "pattern": "^(.*)$"
              },
              "os_type": {
                "$id": "#/properties/nodes/items/properties/hardware/properties/os_type",
                "type": "string",
                "title": "os_type",
                "enum": ["windows", "linux", "rhel", "centos"],
                "default": "linux",
                "examples": [
                  "windows",
                  "linux",
                  "rhel",
                  "centos"
                ],
                "pattern": "^(.*)$"
              },
              "drives": {
                "$id": "#/properties/nodes/items/properties/hardware/properties/drives",
                "type": "array",
                "title": "Drives",
                "items": {
                  "$id": "#/properties/nodes/items/properties/hardware/properties/drives/items",
                  "type": "object",
                  "title": "Drive",
                  "required": [
                    "image"
                  ],
                  "properties": {
                    "image": {
                      "$id": "#/properties/nodes/items/properties/hardware/properties/drives/items/properties/image",
                      "type": "string",
                      "title": "Image",
                      "minLength": 1,
                      "default": "",
                      "examples": [
                        "win10provider.qc2"
                      ],
                      "pattern": "^(.*)$"
                    },
                    "interface": {
                      "$id": "#/properties/nodes/items/properties/hardware/properties/drives/items/properties/interface",
                      "type": "string",
                      "title": "interface",
                      "enum": ["ahci", "ide", "scsi", "sd", "mtd", "floppy", "pflash", "virtio"],
                      "default": "ide",
                      "examples": [
                        "ide"
                      ],
                      "pattern": "^(.*)$"
                    },
                    "cache_mode": {
                      "$id": "#/properties/nodes/items/properties/hardware/properties/drives/items/properties/cache_mode",
                      "type": "string",
                      "title": "cache_mode",
                      "enum": ["none", "writeback", "unsafe", "directsync", "writethrough"],
                      "default": "writeback",
                      "examples": [
                        "writeback"
                      ],
                      "pattern": "^(.*)$"
                    },
                    "inject_partition": {
                      "$id": "#/properties/nodes/items/properties/hardware/properties/drives/items/properties/inject_partition",
                      "type": "string",
                      "title": "inject_partition",
                      "default": "1",
                      "examples": [
                        "2"
                      ],
                      "pattern": "^(.*)$"
                    }
                  }
                }
              }
            }
          },
          "network": {
            "$id": "#/properties/nodes/items/properties/network",
            "type": "object",
            "title": "Network",
            "required": [
              "interfaces"
            ],
            "properties": {
              "interfaces": {
                "$id": "#/properties/nodes/items/properties/network/properties/interfaces",
                "type": "array",
                "title": "Interfaces",
                "items": {
                  "$id": "#/properties/nodes/items/properties/network/properties/interfaces/items",
                  "type": "object",
                  "title": "Interface",
                  "oneOf": [
                    {
                      "$ref": "#/definitions/static_iface",
                      "title": "Static"
                    },
                    {
                      "$ref": "#/definitions/dhcp_iface",
                      "title": "DHCP"
                    },
                    {
                      "$ref": "#/definitions/serial_iface",
                      "title": "Serial"
                    }
                  ]
                }
              },
              "routes": {
                "$id": "#/properties/nodes/items/properties/network/properties/routes",
                "type": "array",
                "items": {
                  "$id": "#/properties/nodes/items/properties/network/properties/routes/items",
                  "type": "object",
                  "title": "Route",
                  "required": [
                    "destination",
                    "next",
                    "cost"
                  ],
                  "properties": {
                    "destination": {
                      "$id": "#/properties/nodes/items/properties/network/properties/routes/items/properties/destination",
                      "type": "string",
                      "title": "Destination",
                      "minLength": 1,
                      "default": "",
                      "examples": [
                        "192.168.0.0/24"
                      ],
                      "pattern": "^(.*)$"
                    },
                    "next": {
                      "$id": "#/properties/nodes/items/properties/network/properties/routes/items/properties/next",
                      "type": "string",
                      "title": "Next",
                      "minLength": 1,
                      "default": "",
                      "examples": [
                        "192.168.1.254"
                      ],
                      "pattern": "^(.*)$"
                    },
                    "cost": {
                      "$id": "#/properties/nodes/items/properties/network/properties/routes/items/properties/cost",
                      "type": "string",
                      "title": "Cost",
                      "minLength": 1,
                      "default": "1",
                      "examples": [
                        "1"
                      ],
                      "pattern": "^(.*)$"
                    }
                  }
                }
              },
              "ospf": {
                "$id": "#/properties/nodes/items/properties/network/properties/ospf",
                "type": "object",
                "title": "Ospf",
                "required": [
                  "router_id",
                  "areas"
                ],
                "properties": {
                  "router_id": {
                    "$id": "#/properties/nodes/items/properties/network/properties/ospf/properties/router_id",
                    "type": "string",
                    "title": "Router_id",
                    "default": "",
                    "examples": [
                      "0.0.0.1"
                    ],
                    "pattern": "^(.*)$"
                  },
                  "areas": {
                    "$id": "#/properties/nodes/items/properties/network/properties/ospf/properties/areas",
                    "type": "array",
                    "title": "Areas",
                    "items": {
                      "$id": "#/properties/nodes/items/properties/network/properties/ospf/properties/areas/items",
                      "type": "object",
                      "title": "Area",
                      "required": [
                        "area_id",
                        "area_networks"
                      ],
                      "properties": {
                        "area_id": {
                          "$id": "#/properties/nodes/items/properties/network/properties/ospf/properties/areas/items/properties/area_id",
                          "type": "string",
                          "title": "Area_id",
                          "default": "",
                          "examples": [
                            "0"
                          ],
                          "pattern": "^(.*)$"
                        },
                        "area_networks": {
                          "$id": "#/properties/nodes/items/properties/network/properties/ospf/properties/areas/items/properties/area_networks",
                          "type": "array",
                          "title": "Area_networks",
                          "items": {
                            "$id": "#/properties/nodes/items/properties/network/properties/ospf/properties/areas/items/properties/area_networks/items",
                            "type": "object",
                            "title": "Area Network",
                            "required": [
                              "network"
                            ],
                            "properties": {
                              "network": {
                                "$id": "#/properties/nodes/items/properties/network/properties/ospf/properties/areas/items/properties/area_networks/items/properties/network",
                                "type": "string",
                                "title": "Network",
                                "default": "",
                                "examples": [
                                  "10.1.25.0/24"
                                ],
                                "pattern": "^(.*)$"
                              }
                            }
                          }
                        }
                      }
                    }
                  }
                }
              },
              "rulesets": {
                "$id": "#/properties/nodes/items/properties/network/properties/rulesets",
                "type": "array",
                "title": "Rulesets",
                "items": {
                  "$id": "#/properties/nodes/items/properties/network/properties/rulesets/items",
                  "type": "object",
                  "title": "Ruleset",
                  "required": [
                    "name",
                    "default",
                    "rules"
                  ],
                  "properties": {
                    "name": {
                      "$id": "#/properties/nodes/items/properties/network/properties/rulesets/items/properties/name",
                      "type": "string",
                      "title": "Name",
                      "minLength": 1,
                      "default": "",
                      "examples": [
                        "OutToDMZ"
                      ],
                      "pattern": "^(.*)$"
                    },
                    "description": {
                      "$id": "#/properties/nodes/items/properties/network/properties/rulesets/items/properties/description",
                      "type": "string",
                      "title": "Description",
                      "default": "",
                      "examples": [
                        "From ICS to the DMZ network"
                      ],
                      "pattern": "^(.*)$"
                    },
                    "default": {
                      "$id": "#/properties/nodes/items/properties/network/properties/rulesets/items/properties/default",
                      "type": "string",
                      "title": "Default",
                      "minLength": 1,
                      "default": "",
                      "examples": [
                        "drop"
                      ],
                      "pattern": "^(.*)$"
                    },
                    "rules": {
                      "$id": "#/properties/nodes/items/properties/network/properties/rulesets/items/properties/rules",
                      "type": "array",
                      "title": "Rules",
                      "items": {
                        "$id": "#/properties/nodes/items/properties/network/properties/rulesets/items/properties/rules/items",
                        "type": "object",
                        "title": "Rule",
                        "required": [
                          "id",
                          "action",
                          "protocol"
                        ],
                        "properties": {
                          "id": {
                            "$id": "#/properties/nodes/items/properties/network/properties/rulesets/items/properties/rules/items/properties/id",
                            "type": "string",
                            "title": "Id",
                            "minLength": 1,
                            "default": "",
                            "examples": [
                              "10"
                            ],
                            "pattern": "^(.*)$"
                          },
                          "description": {
                            "$id": "#/properties/nodes/items/properties/network/properties/rulesets/items/properties/rules/items/properties/description",
                            "type": "string",
                            "title": "Description",
                            "default": "",
                            "examples": [
                              "Allow UDP 10.1.26.80 ==> 10.2.25.0/24:123"
                            ],
                            "pattern": "^(.*)$"
                          },
                          "action": {
                            "$id": "#/properties/nodes/items/properties/network/properties/rulesets/items/properties/rules/items/properties/action",
                            "type": "string",
                            "title": "Action",
                             "enum": [
                              "accept",
                              "drop",
                              "reject"
                            ],
                            "default": "drop",
                            "examples": [
                              "accept",
                              "drop",
                              "reject"
                            ],
                            "pattern": "^(.*)$"
                          },
                          "protocol": {
                            "$id": "#/properties/nodes/items/properties/network/properties/rulesets/items/properties/rules/items/properties/protocol",
                            "type": "string",
                            "title": "Protocol",
                            "enum": [
                              "tcp",
                              "udp",
                              "icmp",
                              "all"
                            ],
                            "default": "tcp",
                            "examples": [
                              "tcp",
                              "udp",
                              "icmp",
                              "all"
                            ],
                            "pattern": "^(.*)$"
                          },
                          "source": {
                            "$id": "#/properties/nodes/items/properties/network/properties/rulesets/items/properties/rules/items/properties/source",
                            "type": "object",
                            "title": "Source",
                            "required": [
                              "address"
                            ],
                            "properties": {
                              "address": {
                                "$id": "#/properties/nodes/items/properties/network/properties/rulesets/items/properties/rules/items/properties/source/properties/address",
                                "type": "string",
                                "title": "Address",
                                "default": "",
                                "examples": [
                                  "10.1.24.0/24",
                                  "10.1.24.60"
                                ],
                                "pattern": "^(.*)$"
                              },
                              "port": {
                                "$id": "#/properties/nodes/items/properties/network/properties/rulesets/items/properties/rules/items/properties/source/properties/port",
                                "type": "string",
                                "title": "Port",
                                "default": "",
                                "examples": [
                                  "3389"
                                ],
                                "pattern": "^(.*)$"
                              }
                            }
                          },
                          "destination": {
                            "$id": "#/properties/nodes/items/properties/network/properties/rulesets/items/properties/rules/items/properties/destination",
                            "type": "object",
                            "title": "Destination",
                            "required": [
                              "address"
                            ],
                            "properties": {
                              "address": {
                                "$id": "#/properties/nodes/items/properties/network/properties/rulesets/items/properties/rules/items/properties/source/properties/address",
                                "type": "string",
                                "title": "Address",
                                "default": "",
                                "examples": [
                                  "10.1.24.0/24",
                                  "10.1.24.60"
                                ],
                                "pattern": "^(.*)$"
                              },
                              "port": {
                                "$id": "#/properties/nodes/items/properties/network/properties/rulesets/items/properties/rules/items/properties/destination/properties/port",
                                "type": "string",
                                "title": "Port",
                                "default": "",
                                "examples": [
                                  "3389"
                                ],
                                "pattern": "^(.*)$"
                              }
                            }
                          }
                        }
                      }
                    }
                  }
                }
              }
            }
          },
          "injections": {
            "$id": "#/properties/nodes/items/properties/injections",
            "type": "array",
            "title": "Injections",
            "items": {
              "$id": "#/properties/nodes/items/properties/injections/items",
              "type": "object",
              "title": "Injection",
              "required": [
                "src",
                "dst"
              ],
              "properties": {
                "src": {
                  "$id": "#/properties/nodes/items/properties/injections/properties/src",
                  "type": "string",
                  "title": "Src",
                  "minLength": 1,
                  "default": "",
                  "examples": [
                    "ACTIVSg2000.PWB"
                  ],
                  "pattern": "^(.*)$"
                },
                "dst": {
                  "$id": "#/properties/nodes/items/properties/injections/properties/dst",
                  "type": "string",
                  "title": "Dst",
                  "minLength": 1,
                  "default": "",
                  "examples": [
                    "sceptre/ACTIVSg2000.PWB"
                  ],
                  "pattern": "^(.*)$"
                },
                "description": {
                  "$id": "#/properties/nodes/items/properties/injections/properties/description",
                  "type": "string",
                  "title": "description",
                  "default": "",
                  "examples": [
                    "PowerWorld case binary data"
                  ],
                  "pattern": "^(.*)$"
                }
              }
            }
          },
          "metadata": {
            "$id": "#/properties/nodes/items/properties/metadata",
            "type": "object",
            "title": "Metadata",
            "properties": {
              "infrastructure": {
                "$id": "#/properties/nodes/items/properties/metadata/properties/infrastructure",
                "type": "string",
                "title": "Infrastructure",
                "enum": ["power-transmission", "batch-process"],
                "default": "power-transmission",
                "examples": [
                  "power-transmission",
                  "batch-process"
                ],
                "pattern": "^(.*)$"
              },
              "provider": {
                "$id": "#/properties/nodes/items/properties/metadata/properties/provider",
                "type": "string",
                "title": "Provider",
                "default": "power-provider",
                "examples": [
                  "power-provider",
                  "simulink-provider"
                ],
                "pattern": "^(.*)$"
              },
              "simulator": {
                "$id": "#/properties/nodes/items/properties/metadata/properties/simulator",
                "type": "string",
                "title": "Simulator",
                "enum": ["Dummy", "PSSE", "PyPower", "PowerWorld", "PowerWorldDynamics", "OpenDSS", "Simulink"],
                "default": "PowerWorld",
                "examples": [
                  "PowerWorld"
                ],
                "pattern": "^(.*)$"
              },
              "publish_endpoint": {
                "$id": "#/properties/nodes/items/properties/metadata/properties/publish_endpoint",
                "type": "string",
                "title": "Publish_endpoint",
                "default": "udp://*;239.0.0.1:40000",
                "examples": [
                  "udp://*;239.0.0.1:40000"
                ],
                "pattern": "^(.*)$"
              },
              "cycle_time": {
                "$id": "#/properties/nodes/items/properties/metadata/properties/cycle_time",
                "type": "string",
                "title": "Cycle_time",
                "default": "500",
                "examples": [
                  "1000"
                ],
                "pattern": "^(.*)$"
              },
              "dnp3": {
                "$id": "#/properties/nodes/items/properties/metadata/properties/dnp3",
                "type": "array",
                "title": "Dnp3",
                "items": {
                  "$id": "#/properties/nodes/items/properties/metadata/properties/dnp3/items",
                  "type": "object",
                  "title": "DNP3 Metadata",
                  "required": [
                    "type",
                    "name"
                  ],
                  "properties": {
                    "type": {
                      "$id": "#/properties/nodes/items/properties/metadata/properties/dnp3/items/properties/type",
                      "type": "string",
                      "title": "Type",
                      "default": "",
                      "examples": [
                        "bus"
                      ],
                      "pattern": "^(.*)$"
                    },
                    "name": {
                      "$id": "#/properties/nodes/items/properties/metadata/properties/dnp3/items/properties/name",
                      "type": "string",
                      "title": "Name",
                      "default": "",
                      "examples": [
                        "bus-2052"
                      ],
                      "pattern": "^(.*)$"
                    },"analog-read": {
                      "$id": "#/properties/nodes/items/properties/metadata/properties/dnp3/items/properties/analog-read",
                      "type": "array",
                      "title": "The Analog-read Schema",
                      "items": {
                        "$id": "#/properties/metadata/properties/dnp3/items/properties/analog-read/items",
                        "type": "object",
                        "title": "The Items Schema",
                        "required": [
                          "field",
                          "register_number",
                          "register_type"
                        ],
                        "properties": {
                          "field": {
                            "$id": "#/properties/nodes/items/properties/metadata/properties/dnp3/items/properties/analog-read/items/properties/field",
                            "type": "string",
                            "title": "The Field Schema",
                            "default": "",
                            "examples": [
                              "voltage"
                            ],
                            "pattern": "^(.*)$"
                          },
                          "register_number": {
                            "$id": "#/properties/nodes/items/properties/metadata/properties/dnp3/items/properties/analog-read/items/properties/register_number",
                            "type": "integer",
                            "title": "The Register_number Schema",
                            "default": 0,
                            "examples": [
                              0
                            ]
                          },
                          "register_type": {
                            "$id": "#/properties/nodes/items/properties/metadata/properties/dnp3/items/properties/analog-read/items/properties/register_type",
                            "type": "string",
                            "title": "The Register_type Schema",
                            "default": "",
                            "examples": [
                              "analog-input"
                            ],
                            "pattern": "^(.*)$"
                          }
                        }
                      }
                    },
                    "binary-read": {
                      "$id": "#/properties/nodes/items/properties/metadata/properties/dnp3/items/properties/binary-read",
                      "type": "array",
                      "title": "The Binary-read Schema",
                      "items": {
                        "$id": "#/properties/metadata/properties/dnp3/items/properties/binary-read/items",
                        "type": "object",
                        "title": "The Items Schema",
                        "required": [
                          "field",
                          "register_number",
                          "register_type"
                        ],
                        "properties": {
                          "field": {
                            "$id": "#/properties/nodes/items/properties/metadata/properties/dnp3/items/properties/binary-read/items/properties/field",
                            "type": "string",
                            "title": "The Field Schema",
                            "default": "",
                            "examples": [
                              "active"
                            ],
                            "pattern": "^(.*)$"
                          },
                          "register_number": {
                            "$id": "#/properties/nodes/items/properties/metadata/properties/dnp3/items/properties/binary-read/items/properties/register_number",
                            "type": "integer",
                            "title": "The Register_number Schema",
                            "default": 0,
                            "examples": [
                              7
                            ]
                          },
                          "register_type": {
                            "$id": "#/properties/nodes/items/properties/metadata/properties/dnp3/items/properties/binary-read/items/properties/register_type",
                            "type": "string",
                            "title": "The Register_type Schema",
                            "default": "",
                            "examples": [
                              "binary-input"
                            ],
                            "pattern": "^(.*)$"
                          }
                        }
                      }
                    },
                    "binary-read-write": {
                      "$id": "#/properties/nodes/items/properties/metadata/properties/dnp3/items/properties/binary-read-write",
                      "type": "array",
                      "title": "The Binary-read-write Schema",
                      "items": {
                        "$id": "#/properties/metadata/properties/dnp3/items/properties/binary-read-write/items",
                        "type": "object",
                        "title": "The Items Schema",
                        "required": [
                          "field",
                          "register_number",
                          "register_type"
                        ],
                        "properties": {
                          "field": {
                            "$id": "#/properties/nodes/items/properties/metadata/properties/dnp3/items/properties/binary-read-write/items/properties/field",
                            "type": "string",
                            "title": "The Field Schema",
                            "default": "",
                            "examples": [
                              "active"
                            ],
                            "pattern": "^(.*)$"
                          },
                          "register_number": {
                            "$id": "#/properties/nodes/items/properties/metadata/properties/dnp3/items/properties/binary-read-write/items/properties/register_number",
                            "type": "integer",
                            "title": "The Register_number Schema",
                            "default": 0,
                            "examples": [
                              9
                            ]
                          },
                          "register_type": {
                            "$id": "#/properties/nodes/items/properties/metadata/properties/dnp3/items/properties/binary-read-write/items/properties/register_type",
                            "type": "string",
                            "title": "The Register_type Schema",
                            "default": "",
                            "examples": [
                              "binary-output"
                            ],
                            "pattern": "^(.*)$"
                          }
                        }
                      }
                    }
                  }
                }
              },
              "dnp3-serial": {
                "$id": "#/properties/nodes/items/properties/metadata/properties/dnp3-serial",
                "type": "array",
                "title": "Dnp3-serial",
                "items": {
                  "$id": "#/properties/nodes/items/properties/metadata/properties/dnp3-serial/items",
                  "type": "object",
                  "title": "DNP3-serial Metadata",
                  "required": [
                    "type",
                    "name"
                  ],
                  "properties": {
                    "type": {
                      "$id": "#/properties/nodes/items/properties/metadata/properties/dnp3-serial/items/properties/type",
                      "type": "string",
                      "title": "Type",
                      "default": "",
                      "examples": [
                        "bus"
                      ],
                      "pattern": "^(.*)$"
                    },
                    "name": {
                      "$id": "#/properties/nodes/items/properties/metadata/properties/dnp3-serial/items/properties/name",
                      "type": "string",
                      "title": "Name",
                      "default": "",
                      "examples": [
                        "bus-2052"
                      ],
                      "pattern": "^(.*)$"
                    },"analog-read": {
                      "$id": "#/properties/nodes/items/properties/metadata/properties/dnp3-serial/items/properties/analog-read",
                      "type": "array",
                      "title": "The Analog-read Schema",
                      "items": {
                        "$id": "#/properties/metadata/properties/dnp3-serial/items/properties/analog-read/items",
                        "type": "object",
                        "title": "The Items Schema",
                        "required": [
                          "field",
                          "register_number",
                          "register_type"
                        ],
                        "properties": {
                          "field": {
                            "$id": "#/properties/nodes/items/properties/metadata/properties/dnp3-serial/items/properties/analog-read/items/properties/field",
                            "type": "string",
                            "title": "The Field Schema",
                            "default": "",
                            "examples": [
                              "voltage"
                            ],
                            "pattern": "^(.*)$"
                          },
                          "register_number": {
                            "$id": "#/properties/nodes/items/properties/metadata/properties/dnp3-serial/items/properties/analog-read/items/properties/register_number",
                            "type": "integer",
                            "title": "The Register_number Schema",
                            "default": 0,
                            "examples": [
                              0
                            ]
                          },
                          "register_type": {
                            "$id": "#/properties/nodes/items/properties/metadata/properties/dnp3-serial/items/properties/analog-read/items/properties/register_type",
                            "type": "string",
                            "title": "The Register_type Schema",
                            "default": "",
                            "examples": [
                              "analog-input"
                            ],
                            "pattern": "^(.*)$"
                          }
                        }
                      }
                    },
                    "binary-read": {
                      "$id": "#/properties/nodes/items/properties/metadata/properties/dnp3-serial/items/properties/binary-read",
                      "type": "array",
                      "title": "The Binary-read Schema",
                      "items": {
                        "$id": "#/properties/metadata/properties/dnp3-serial/items/properties/binary-read/items",
                        "type": "object",
                        "title": "The Items Schema",
                        "required": [
                          "field",
                          "register_number",
                          "register_type"
                        ],
                        "properties": {
                          "field": {
                            "$id": "#/properties/nodes/items/properties/metadata/properties/dnp3-serial/items/properties/binary-read/items/properties/field",
                            "type": "string",
                            "title": "The Field Schema",
                            "default": "",
                            "examples": [
                              "active"
                            ],
                            "pattern": "^(.*)$"
                          },
                          "register_number": {
                            "$id": "#/properties/nodes/items/properties/metadata/properties/dnp3-serial/items/properties/binary-read/items/properties/register_number",
                            "type": "integer",
                            "title": "The Register_number Schema",
                            "default": 0,
                            "examples": [
                              7
                            ]
                          },
                          "register_type": {
                            "$id": "#/properties/nodes/items/properties/metadata/properties/dnp3-serial/items/properties/binary-read/items/properties/register_type",
                            "type": "string",
                            "title": "The Register_type Schema",
                            "default": "",
                            "examples": [
                              "binary-input"
                            ],
                            "pattern": "^(.*)$"
                          }
                        }
                      }
                    },
                    "binary-read-write": {
                      "$id": "#/properties/nodes/items/properties/metadata/properties/dnp3-serial/items/properties/binary-read-write",
                      "type": "array",
                      "title": "The Binary-read-write Schema",
                      "items": {
                        "$id": "#/properties/metadata/properties/dnp3-serial/items/properties/binary-read-write/items",
                        "type": "object",
                        "title": "The Items Schema",
                        "required": [
                          "field",
                          "register_number",
                          "register_type"
                        ],
                        "properties": {
                          "field": {
                            "$id": "#/properties/nodes/items/properties/metadata/properties/dnp3-serial/items/properties/binary-read-write/items/properties/field",
                            "type": "string",
                            "title": "The Field Schema",
                            "default": "",
                            "examples": [
                              "active"
                            ],
                            "pattern": "^(.*)$"
                          },
                          "register_number": {
                            "$id": "#/properties/nodes/items/properties/metadata/properties/dnp3-serial/items/properties/binary-read-write/items/properties/register_number",
                            "type": "integer",
                            "title": "The Register_number Schema",
                            "default": 0,
                            "examples": [
                              9
                            ]
                          },
                          "register_type": {
                            "$id": "#/properties/nodes/items/properties/metadata/properties/dnp3-serial/items/properties/binary-read-write/items/properties/register_type",
                            "type": "string",
                            "title": "The Register_type Schema",
                            "default": "",
                            "examples": [
                              "binary-output"
                            ],
                            "pattern": "^(.*)$"
                          }
                        }
                      }
                    }
                  }
                }
              },
              "modbus": {
                "$id": "#/properties/nodes/items/properties/metadata/properties/modbus",
                "type": "array",
                "title": "Modbus",
                "items": {
                  "$id": "#/properties/nodes/items/properties/metadata/properties/modbus/items",
                  "type": "object",
                  "title": "Modbus Metadata",
                  "required": [
                    "type",
                    "name"
                  ],
                  "properties": {
                    "type": {
                      "$id": "#/properties/nodes/items/properties/metadata/properties/modbus/items/properties/type",
                      "type": "string",
                      "title": "Type",
                      "default": "",
                      "examples": [
                        "bus"
                      ],
                      "pattern": "^(.*)$"
                    },
                    "name": {
                      "$id": "#/properties/nodes/items/properties/metadata/properties/modbus/items/properties/name",
                      "type": "string",
                      "title": "Name",
                      "default": "",
                      "examples": [
                        "bus-2052"
                      ],
                      "pattern": "^(.*)$"
                    },
                    "analog-read": {
                      "$id": "#/properties/nodes/items/properties/metadata/properties/modbus/items/properties/analog-read",
                      "type": "array",
                      "title": "The Analog-read Schema",
                      "items": {
                        "$id": "#/properties/nodes/items/properties/metadata/properties/modbus/items/properties/analog-read/items",
                        "type": "object",
                        "title": "The Items Schema",
                        "required": [
                          "field",
                          "register_number",
                          "register_type"
                        ],
                        "properties": {
                          "field": {
                            "$id": "#/properties/nodes/items/properties/metadata/properties/modbus/items/properties/analog-read/items/properties/field",
                            "type": "string",
                            "title": "The Field Schema",
                            "default": "",
                            "examples": [
                              "voltage"
                            ],
                            "pattern": "^(.*)$"
                          },
                          "register_number": {
                            "$id": "#/properties/nodes/items/properties/metadata/properties/modbus/items/properties/analog-read/items/properties/register_number",
                            "type": "integer",
                            "title": "The Register_number Schema",
                            "default": 0,
                            "examples": [
                              30000
                            ]
                          },
                          "register_type": {
                            "$id": "#/properties/nodes/items/properties/metadata/properties/modbus/items/properties/analog-read/items/properties/register_type",
                            "type": "string",
                            "title": "The Register_type Schema",
                            "default": "",
                            "examples": [
                              "input-register"
                            ],
                            "pattern": "^(.*)$"
                          }
                        }
                      }
                    },
                    "binary-read": {
                      "$id": "#/properties/nodes/items/properties/metadata/properties/modbus/items/properties/binary-read",
                      "type": "array",
                      "title": "The Binary-read Schema",
                      "items": {
                        "$id": "#/properties/metadata/properties/modbus/items/properties/binary-read/items",
                        "type": "object",
                        "title": "The Items Schema",
                        "required": [
                          "field",
                          "register_number",
                          "register_type"
                        ],
                        "properties": {
                          "field": {
                            "$id": "#/properties/nodes/items/properties/metadata/properties/modbus/items/properties/binary-read/items/properties/field",
                            "type": "string",
                            "title": "The Field Schema",
                            "default": "",
                            "examples": [
                              "active"
                            ],
                            "pattern": "^(.*)$"
                          },
                          "register_number": {
                            "$id": "#/properties/nodes/items/properties/metadata/properties/modbus/items/properties/binary-read/items/properties/register_number",
                            "type": "integer",
                            "title": "The Register_number Schema",
                            "default": 0,
                            "examples": [
                              10000
                            ]
                          },
                          "register_type": {
                            "$id": "#/properties/nodes/items/properties/metadata/properties/modbus/items/properties/binary-read/items/properties/register_type",
                            "type": "string",
                            "title": "The Register_type Schema",
                            "default": "",
                            "examples": [
                              "discrete-input"
                            ],
                            "pattern": "^(.*)$"
                          }
                        }
                      }
                    },
                    "binary-read-write": {
                      "$id": "#/properties/nodes/items/properties/metadata/properties/modbus/items/properties/binary-read-write",
                      "type": "array",
                      "title": "The Binary-read-write Schema",
                      "items": {
                        "$id": "#/properties/metadata/properties/modbus/items/properties/binary-read-write/items",
                        "type": "object",
                        "title": "The Items Schema",
                        "required": [
                          "field",
                          "register_number",
                          "register_type"
                        ],
                        "properties": {
                          "field": {
                            "$id": "#/properties/nodes/items/properties/metadata/properties/modbus/items/properties/binary-read-write/items/properties/field",
                            "type": "string",
                            "title": "The Field Schema",
                            "default": "",
                            "examples": [
                              "active"
                            ],
                            "pattern": "^(.*)$"
                          },
                          "register_number": {
                            "$id": "#/properties/nodes/items/properties/metadata/properties/modbus/items/properties/binary-read-write/items/properties/register_number",
                            "type": "integer",
                            "title": "The Register_number Schema",
                            "default": 0,
                            "examples": [
                              0
                            ]
                          },
                          "register_type": {
                            "$id": "#/properties/nodes/items/properties/metadata/properties/modbus/items/properties/binary-read-write/items/properties/register_type",
                            "type": "string",
                            "title": "The Register_type Schema",
                            "default": "",
                            "examples": [
                              "coil"
                            ],
                            "pattern": "^(.*)$"
                          }
                        }
                      }
                    }
                  }
                }
              },
              "logic": {
                "$id": "#/properties/nodes/items/properties/metadata/properties/logic",
                "type": "string",
                "title": "Logic",
                "default": "",
                "examples": [
                  "Tank1.fill_control = Tank1.tank_volume < Tank1.level_setpoint || (Tank1.tank_volume < 1.5*Tank1.level_setpoint && Tank1.fill_control == 1); Pump1.control = ! FillingStation1.request == 0 && Tank1.tank_volume>0; Pump1.active = 1==1"
                ],
                "pattern": "^(.*)$"
              },
              "connected_rtus": {
                "$id": "#/properties/nodes/items/properties/metadata/properties/connected_rtus",
                "type": "array",
                "title": "Connected RTUs",
                "items": {
                  "$id": "#/properties/nodes/items/properties/metadata/properties/connected_rtus/items",
                  "type": "string",
                  "title": "RTU",
                  "default": "",
                  "examples": [
                    "rtu-1",
                    "fuel-rtu-2"
                  ],
                  "pattern": "^(.*)$"
                }
              },
              "connect_to_scada": {
                "type": "boolean",
                "title": "connect_to_scada",
                "default": false,
                "examples": [
                  true,
                  false
                ]
              },
              "manual_register_config": {
                "$id": "#/properties/nodes/items/properties/metadata/properties/manual_register_config",
                "type": "string",
                "title": "Manual register configuration",
                "default": "false",
                "examples": [
                  "false"
                ]
              }
            }
          }
        }
}


    var graph = ui.editor.graph;
    var value = graph.getModel().getValue(cell);
    console.log('this is cell and  getValue(cell):'), console.log(cell), console.log(value);
    const type = cell.isVertex() ? 'nodes' : 'edges';
    var nodes = ui.topoJSON[type];

    var id = (EditDataDialog.getDisplayIdForCell != null) ?
        EditDataDialog.getDisplayIdForCell(ui, cell) : null;

    // var cellId = cell.id;
    var result = nodes.filter(obj => {
      return obj.id === id;
    });
    console.log(result);

    var node = null;
    if(result.length > 0){
        node = result[0];
    }
    else{
        node = {};
    }
    node.id = id;
    console.log(node);

    const options = {
        schema: schema,
        startval: node,
        // schemaRefs: {"job": job},
        ajax: true,
        mode: 'tree',
        modes: ['code', 'text', 'tree'],
        show_errors: 'always',
        // jsoneditor: {
        //     css: 'utils/topographer/downloads/jsoneditor.min.css',
        //     js: 'utils/topographer/downloads/jsoneditor.js'
        // },
        theme: 'bootstrap3',
        iconlib: 'spectre',
        // template: {
        // },
        ext_lib: {
            lib_dompurify: {
                js: 'utils/topographer/downloads/purify.min.js'
            }
        }
    }

    var editorContainer = document.createElement('div');
    editorContainer.setAttribute('id', 'jsoneditor');
    editorContainer.style.height = '100%';
    editorContainer.style.position = 'relative';
    editorContainer.style['max-width'] ="600px";

    // var css = document.createElement('style');
    // css.setAttribute('scoped', 'scoped');
    // css.innerHTML = '@import "grapheditor/utils/topographer/downloads/spectre.min.css"';
    // editorContainer.appendChild(css);

    // css = document.createElement('style');
    // css.setAttribute('scoped', 'scoped');
    // css.innerHTML = '@import "grapheditor/utils/topographer/downloads/spectre-exp.min.css"';
    // editorContainer.appendChild(css);

    // create the editor
    // const container = document.getElementById('jsoneditor')
    const editor = new JSONEditor(editorContainer, options);

    var div = document.createElement('div');
    div.style['overflow-y'] = 'auto';
    
    // var parameters = {memory:"2048", vcpu:"1", network:{name:'eth1', ip:'127.0.0.1'}, kernel:undefined,initrd:undefined,disk:undefined,snapshot:true,cdrom:undefined};

    function traverse(json, xmlNode) {
        console.log(xmlNode);
        if( json !== null && typeof json == "object" ) {
            Object.entries(json).forEach(([key, value]) => {
                // key is either an array index or object key
                console.log(key);
                console.log(value);
                // var doc = mxUtils.createXmlDocument();
                // var obj = doc.createElement('object');
                if(typeof value === 'object') {
                    xmlNode.setAttribute(key, '');
                    Object.keys(value).forEach(([k, v]) => {
                        var doc = mxUtils.createXmlDocument();
                        var obj = doc.createElement('object');
                        if(typeof v !== 'object') {
                            xmlNode.getElementsByTagName(key).appendChild(obj.setAttribute(k,v));
                        }
                        else{
                            xmlNode.getElementsByTagName(key).appendChild(obj.setAttribute(k,''));
                            traverse(v, xmlNode.getElementsByTagName(key).getElementsByTagName(k));
                        }
                    });
                }
                else {
                    xmlNode.setAttribute(key, value);
                    traverse(value, xmlNode);
                }
            });
        }
        else {
            // jsonObj is a number or string
            console.log('string'), console.log(json);
        }
        return xmlNode;
    }
    
    // Converts the value to an XML node
    if (!mxUtils.isNode(value))
    {
        var doc = mxUtils.createXmlDocument();
        var obj = doc.createElement('object');
        // obj.setAttribute('label', value || '');
        value = obj;
    }

    div.appendChild(editorContainer);

    var cancelBtn = mxUtils.button(mxResources.get('cancel'), function()
    {
        ui.hideDialog.apply(ui, arguments);
        editor.destroy();
    });
    
    cancelBtn.className = 'geBtn';
    
    var applyBtn = mxUtils.button(mxResources.get('apply'), function()
    {
        try
        {
            ui.hideDialog.apply(ui, arguments);
            
            // Clones and updates the value
            value = value.cloneNode(true);
            console.log('this is value in apply'), console.log(value);
            // value = traverse(obj, value);
            
            // for (var i = 0; i < names.length; i++)
            // {
            //     if (texts[i] == null)
            //     {
            //         value.removeAttribute(names[i]);
            //     }
            //     else
            //     {
            //         value.setAttribute(names[i], texts[i].value);
            //         removeLabel = removeLabel || (names[i] == 'placeholder' &&
            //             value.getAttribute('placeholders') == '1');

            //         // TEST
            //         // value.attributes.network = {};
            //         // value.attributes.network.value = {a:1,b:2};
            //     }
            // }
            
            // Updates the value of the cell (undoable)
            // console.log('value after traverse'), console.log(value);
            // graph.getModel().setValue(cell, value);

            // Updates the global ui.topoJSON
            var updatedNode = editor.getEditor('root').value; // get current node's JSON
            console.log('updated node'), console.log(updatedNode);
            value.attributes.topo = {};
            value.attributes.topo.value = updatedNode;
            graph.getModel().setValue(cell, value);
            console.log('value after updated Node'), console.log(value), console.log(cell);
            
            console.log('ui.topoJSON before updates'), console.log(JSON.stringify(ui.topoJSON));
            ui.updateTopoJSON(cell);
            console.log('ui.topoJSON afte updates'), console.log(ui.topoJSON);

        }
        catch (e)
        {
            mxUtils.alert(e);
        }

        editor.destroy();

    });

    applyBtn.className = 'geBtn gePrimaryBtn';
    
    var buttons = document.createElement('div');
    buttons.style.cssText = 'position:absolute;left:30px;right:30px;text-align:right;bottom:15px;height:40px;border-top:1px solid #ccc;padding-top:20px;'
    
    if (ui.editor.cancelFirst)
    {
        buttons.appendChild(cancelBtn);
        buttons.appendChild(applyBtn);
    }
    else
    {
        buttons.appendChild(applyBtn);
        buttons.appendChild(cancelBtn);
    }

    div.appendChild(buttons);
    this.container = div;

    this.init = function()
    {
        console.log('init');
    };

};

/**
 * Optional help link.
 */
EditDataDialog.getDisplayIdForCell = function(ui, cell)
{
    var id = null;
    
    if (ui.editor.graph.getModel().getParent(cell) != null)
    {
        id = cell.getId();
    }
    
    return id;
};

/**
 * Optional help link.
 */
EditDataDialog.placeholderHelpLink = null;

/**
 * Constructs a new link dialog.
 */
var LinkDialog = function(editorUi, initialValue, btnLabel, fn)
{
    var div = document.createElement('div');
    mxUtils.write(div, mxResources.get('editLink') + ':');
    
    var inner = document.createElement('div');
    inner.className = 'geTitle';
    inner.style.backgroundColor = 'transparent';
    inner.style.borderColor = 'transparent';
    inner.style.whiteSpace = 'nowrap';
    inner.style.textOverflow = 'clip';
    inner.style.cursor = 'default';
    
    if (!mxClient.IS_VML)
    {
        inner.style.paddingRight = '20px';
    }
    
    var linkInput = document.createElement('input');
    linkInput.setAttribute('value', initialValue);
    linkInput.setAttribute('placeholder', 'http://www.example.com/');
    linkInput.setAttribute('type', 'text');
    linkInput.style.marginTop = '6px';
    linkInput.style.width = '400px';
    linkInput.style.backgroundImage = 'url(\'' + Dialog.prototype.clearImage + '\')';
    linkInput.style.backgroundRepeat = 'no-repeat';
    linkInput.style.backgroundPosition = '100% 50%';
    linkInput.style.paddingRight = '14px';
    
    var cross = document.createElement('div');
    cross.setAttribute('title', mxResources.get('reset'));
    cross.style.position = 'relative';
    cross.style.left = '-16px';
    cross.style.width = '12px';
    cross.style.height = '14px';
    cross.style.cursor = 'pointer';

    // Workaround for inline-block not supported in IE
    cross.style.display = (mxClient.IS_VML) ? 'inline' : 'inline-block';
    cross.style.top = ((mxClient.IS_VML) ? 0 : 3) + 'px';
    
    // Needed to block event transparency in IE
    cross.style.background = 'url(' + IMAGE_PATH + '/transparent.gif)';

    mxEvent.addListener(cross, 'click', function()
    {
        linkInput.value = '';
        linkInput.focus();
    });
    
    inner.appendChild(linkInput);
    inner.appendChild(cross);
    div.appendChild(inner);
    
    this.init = function()
    {
        linkInput.focus();
        
        if (mxClient.IS_GC || mxClient.IS_FF || document.documentMode >= 5 || mxClient.IS_QUIRKS)
        {
            linkInput.select();
        }
        else
        {
            document.execCommand('selectAll', false, null);
        }
    };
    
    var btns = document.createElement('div');
    btns.style.marginTop = '18px';
    btns.style.textAlign = 'right';

    mxEvent.addListener(linkInput, 'keypress', function(e)
    {
        if (e.keyCode == 13)
        {
            editorUi.hideDialog();
            fn(linkInput.value);
        }
    });

    var cancelBtn = mxUtils.button(mxResources.get('cancel'), function()
    {
        editorUi.hideDialog();
    });
    cancelBtn.className = 'geBtn';
    
    if (editorUi.editor.cancelFirst)
    {
        btns.appendChild(cancelBtn);
    }
    
    var mainBtn = mxUtils.button(btnLabel, function()
    {
        editorUi.hideDialog();
        fn(linkInput.value);
    });
    mainBtn.className = 'geBtn gePrimaryBtn';
    btns.appendChild(mainBtn);
    
    if (!editorUi.editor.cancelFirst)
    {
        btns.appendChild(cancelBtn);
    }

    div.appendChild(btns);

    this.container = div;
};

/**
 * 
 */
var OutlineWindow = function(editorUi, x, y, w, h)
{
    var graph = editorUi.editor.graph;

    var div = document.createElement('div');
    div.style.position = 'absolute';
    div.style.width = '100%';
    div.style.height = '100%';
    div.style.border = '1px solid whiteSmoke';
    div.style.overflow = 'hidden';

    this.window = new mxWindow(mxResources.get('outline'), div, x, y, w, h, true, true);
    this.window.minimumSize = new mxRectangle(0, 0, 80, 80);
    this.window.destroyOnClose = false;
    this.window.setMaximizable(false);
    this.window.setResizable(true);
    this.window.setClosable(true);
    this.window.setVisible(true);
    
    this.window.setLocation = function(x, y)
    {
        var iw = window.innerWidth || document.body.clientWidth || document.documentElement.clientWidth;
        var ih = window.innerHeight || document.body.clientHeight || document.documentElement.clientHeight;
        
        x = Math.max(0, Math.min(x, iw - this.table.clientWidth));
        y = Math.max(0, Math.min(y, ih - this.table.clientHeight - 48));

        if (this.getX() != x || this.getY() != y)
        {
            mxWindow.prototype.setLocation.apply(this, arguments);
        }
    };
    
    var resizeListener = mxUtils.bind(this, function()
    {
        var x = this.window.getX();
        var y = this.window.getY();
        
        this.window.setLocation(x, y);
    });
    
    mxEvent.addListener(window, 'resize', resizeListener);
    
    var outline = editorUi.createOutline(this.window);

    this.destroy = function()
    {
        mxEvent.removeListener(window, 'resize', resizeListener);
        this.window.destroy();
        outline.destroy();
    }

    this.window.addListener(mxEvent.RESIZE, mxUtils.bind(this, function()
    {
        outline.update(false);
        outline.outline.sizeDidChange();
    }));
    
    this.window.addListener(mxEvent.SHOW, mxUtils.bind(this, function()
    {
        this.window.fit();
        outline.suspended = false;
        outline.outline.refresh();
        outline.update();
    }));
    
    this.window.addListener(mxEvent.HIDE, mxUtils.bind(this, function()
    {
        outline.suspended = true;
    }));
    
    this.window.addListener(mxEvent.NORMALIZE, mxUtils.bind(this, function()
    {
        outline.suspended = false;
        outline.update();
    }));
            
    this.window.addListener(mxEvent.MINIMIZE, mxUtils.bind(this, function()
    {
        outline.suspended = true;
    }));

    var outlineCreateGraph = outline.createGraph;
    outline.createGraph = function(container)
    {
        var g = outlineCreateGraph.apply(this, arguments);
        g.gridEnabled = false;
        g.pageScale = graph.pageScale;
        g.pageFormat = graph.pageFormat;
        g.background = (graph.background == null || graph.background == mxConstants.NONE) ? graph.defaultPageBackgroundColor : graph.background;
        g.pageVisible = graph.pageVisible;

        var current = mxUtils.getCurrentStyle(graph.container);
        div.style.backgroundColor = current.backgroundColor;
        
        return g;
    };
    
    function update()
    {
        outline.outline.pageScale = graph.pageScale;
        outline.outline.pageFormat = graph.pageFormat;
        outline.outline.pageVisible = graph.pageVisible;
        outline.outline.background = (graph.background == null || graph.background == mxConstants.NONE) ? graph.defaultPageBackgroundColor : graph.background;;
        
        var current = mxUtils.getCurrentStyle(graph.container);
        div.style.backgroundColor = current.backgroundColor;

        if (graph.view.backgroundPageShape != null && outline.outline.view.backgroundPageShape != null)
        {
            outline.outline.view.backgroundPageShape.fill = graph.view.backgroundPageShape.fill;
        }
        
        outline.outline.refresh();
    };

    outline.init(div);

    editorUi.editor.addListener('resetGraphView', update);
    editorUi.addListener('pageFormatChanged', update);
    editorUi.addListener('backgroundColorChanged', update);
    editorUi.addListener('backgroundImageChanged', update);
    editorUi.addListener('pageViewChanged', function()
    {
        update();
        outline.update(true);
    });
    
    if (outline.outline.dialect == mxConstants.DIALECT_SVG)
    {
        var zoomInAction = editorUi.actions.get('zoomIn');
        var zoomOutAction = editorUi.actions.get('zoomOut');
        
        mxEvent.addMouseWheelListener(function(evt, up)
        {
            var outlineWheel = false;
            var source = mxEvent.getSource(evt);
    
            while (source != null)
            {
                if (source == outline.outline.view.canvas.ownerSVGElement)
                {
                    outlineWheel = true;
                    break;
                }
    
                source = source.parentNode;
            }
    
            if (outlineWheel)
            {
                if (up)
                {
                    zoomInAction.funct();
                }
                else
                {
                    zoomOutAction.funct();
                }
            }
        });
    }
};

/**
 * 
 */
var LayersWindow = function(editorUi, x, y, w, h)
{
    var graph = editorUi.editor.graph;
    
    var div = document.createElement('div');
    div.style.userSelect = 'none';
    div.style.background = (Dialog.backdropColor == 'white') ? 'whiteSmoke' : Dialog.backdropColor;
    div.style.border = '1px solid whiteSmoke';
    div.style.height = '100%';
    div.style.marginBottom = '10px';
    div.style.overflow = 'auto';

    var tbarHeight = (!EditorUi.compactUi) ? '30px' : '26px';
    
    var listDiv = document.createElement('div')
    listDiv.style.backgroundColor = (Dialog.backdropColor == 'white') ? '#dcdcdc' : Dialog.backdropColor;
    listDiv.style.position = 'absolute';
    listDiv.style.overflow = 'auto';
    listDiv.style.left = '0px';
    listDiv.style.right = '0px';
    listDiv.style.top = '0px';
    listDiv.style.bottom = (parseInt(tbarHeight) + 7) + 'px';
    div.appendChild(listDiv);
    
    var dragSource = null;
    var dropIndex = null;
    
    mxEvent.addListener(div, 'dragover', function(evt)
    {
        evt.dataTransfer.dropEffect = 'move';
        dropIndex = 0;
        evt.stopPropagation();
        evt.preventDefault();
    });
    
    // Workaround for "no element found" error in FF
    mxEvent.addListener(div, 'drop', function(evt)
    {
        evt.stopPropagation();
        evt.preventDefault();
    });

    var layerCount = null;
    var selectionLayer = null;
    
    var ldiv = document.createElement('div');
    
    ldiv.className = 'geToolbarContainer';
    ldiv.style.position = 'absolute';
    ldiv.style.bottom = '0px';
    ldiv.style.left = '0px';
    ldiv.style.right = '0px';
    ldiv.style.height = tbarHeight;
    ldiv.style.overflow = 'hidden';
    ldiv.style.padding = (!EditorUi.compactUi) ? '1px' : '4px 0px 3px 0px';
    ldiv.style.backgroundColor = (Dialog.backdropColor == 'white') ? 'whiteSmoke' : Dialog.backdropColor;
    ldiv.style.borderWidth = '1px 0px 0px 0px';
    ldiv.style.borderColor = '#c3c3c3';
    ldiv.style.borderStyle = 'solid';
    ldiv.style.display = 'block';
    ldiv.style.whiteSpace = 'nowrap';
    
    if (mxClient.IS_QUIRKS)
    {
        ldiv.style.filter = 'none';
    }
    
    var link = document.createElement('a');
    link.className = 'geButton';
    
    if (mxClient.IS_QUIRKS)
    {
        link.style.filter = 'none';
    }
    
    var removeLink = link.cloneNode();
    removeLink.innerHTML = '<div class="geSprite geSprite-delete" style="display:inline-block;"></div>';

    mxEvent.addListener(removeLink, 'click', function(evt)
    {
        if (graph.isEnabled())
        {
            graph.model.beginUpdate();
            try
            {
                var index = graph.model.root.getIndex(selectionLayer);
                graph.removeCells([selectionLayer], false);
                
                // Creates default layer if no layer exists
                if (graph.model.getChildCount(graph.model.root) == 0)
                {
                    graph.model.add(graph.model.root, new mxCell());
                    graph.setDefaultParent(null);
                }
                else if (index > 0 && index <= graph.model.getChildCount(graph.model.root))
                {
                    graph.setDefaultParent(graph.model.getChildAt(graph.model.root, index - 1));
                }
                else
                {
                    graph.setDefaultParent(null);
                }
            }
            finally
            {
                graph.model.endUpdate();
            }
        }
        
        mxEvent.consume(evt);
    });
    
    if (!graph.isEnabled())
    {
        removeLink.className = 'geButton mxDisabled';
    }
    
    ldiv.appendChild(removeLink);

    var insertLink = link.cloneNode();
    insertLink.setAttribute('title', mxUtils.trim(mxResources.get('moveSelectionTo', [''])));
    insertLink.innerHTML = '<div class="geSprite geSprite-insert" style="display:inline-block;"></div>';
    
    mxEvent.addListener(insertLink, 'click', function(evt)
    {
        if (graph.isEnabled() && !graph.isSelectionEmpty())
        {
            editorUi.editor.graph.popupMenuHandler.hideMenu();
            
            var menu = new mxPopupMenu(mxUtils.bind(this, function(menu, parent)
            {
                for (var i = layerCount - 1; i >= 0; i--)
                {
                    (mxUtils.bind(this, function(child)
                    {
                        var item = menu.addItem(graph.convertValueToString(child) ||
                                mxResources.get('background'), null, mxUtils.bind(this, function()
                        {
                            graph.moveCells(graph.getSelectionCells(), 0, 0, false, child);
                        }), parent);
                        
                        if (graph.getSelectionCount() == 1 && graph.model.isAncestor(child, graph.getSelectionCell()))
                        {
                            menu.addCheckmark(item, Editor.checkmarkImage);
                        }
                        
                    }))(graph.model.getChildAt(graph.model.root, i));
                }
            }));
            menu.div.className += ' geMenubarMenu';
            menu.smartSeparators = true;
            menu.showDisabled = true;
            menu.autoExpand = true;
            
            // Disables autoexpand and destroys menu when hidden
            menu.hideMenu = mxUtils.bind(this, function()
            {
                mxPopupMenu.prototype.hideMenu.apply(menu, arguments);
                menu.destroy();
            });
    
            var offset = mxUtils.getOffset(insertLink);
            menu.popup(offset.x, offset.y + insertLink.offsetHeight, null, evt);
            
            // Allows hiding by clicking on document
            editorUi.setCurrentMenu(menu);
        }
    });

    ldiv.appendChild(insertLink);
    
    var dataLink = link.cloneNode();
    dataLink.innerHTML = '<div class="geSprite geSprite-dots" style="display:inline-block;"></div>';
    dataLink.setAttribute('title', mxResources.get('rename'));

    mxEvent.addListener(dataLink, 'click', function(evt)
    {
        if (graph.isEnabled())
        {
            editorUi.showDataDialog(selectionLayer);
        }
        
        mxEvent.consume(evt);
    });
    
    if (!graph.isEnabled())
    {
        dataLink.className = 'geButton mxDisabled';
    }

    ldiv.appendChild(dataLink);
    
    function renameLayer(layer)
    {
        if (graph.isEnabled() && layer != null)
        {
            var label = graph.convertValueToString(layer);
            var dlg = new FilenameDialog(editorUi, label || mxResources.get('background'), mxResources.get('rename'), mxUtils.bind(this, function(newValue)
            {
                if (newValue != null)
                {
                    graph.cellLabelChanged(layer, newValue);
                }
            }), mxResources.get('enterName'));
            editorUi.showDialog(dlg.container, 300, 100, true, true);
            dlg.init();
        }
    };
    
    var duplicateLink = link.cloneNode();
    duplicateLink.innerHTML = '<div class="geSprite geSprite-duplicate" style="display:inline-block;"></div>';
    
    mxEvent.addListener(duplicateLink, 'click', function(evt)
    {
        if (graph.isEnabled())
        {
            var newCell = null;
            graph.model.beginUpdate();
            try
            {
                newCell = graph.cloneCell(selectionLayer);
                graph.cellLabelChanged(newCell, mxResources.get('untitledLayer'));
                newCell.setVisible(true);
                newCell = graph.addCell(newCell, graph.model.root);
                graph.setDefaultParent(newCell);
            }
            finally
            {
                graph.model.endUpdate();
            }

            if (newCell != null && !graph.isCellLocked(newCell))
            {
                graph.selectAll(newCell);
            }
        }
    });
    
    if (!graph.isEnabled())
    {
        duplicateLink.className = 'geButton mxDisabled';
    }

    ldiv.appendChild(duplicateLink);

    var addLink = link.cloneNode();
    addLink.innerHTML = '<div class="geSprite geSprite-plus" style="display:inline-block;"></div>';
    addLink.setAttribute('title', mxResources.get('addLayer'));
    
    mxEvent.addListener(addLink, 'click', function(evt)
    {
        if (graph.isEnabled())
        {
            graph.model.beginUpdate();
            
            try
            {
                var cell = graph.addCell(new mxCell(mxResources.get('untitledLayer')), graph.model.root);
                graph.setDefaultParent(cell);
            }
            finally
            {
                graph.model.endUpdate();
            }
        }
        
        mxEvent.consume(evt);
    });
    
    if (!graph.isEnabled())
    {
        addLink.className = 'geButton mxDisabled';
    }
    
    ldiv.appendChild(addLink);

    div.appendChild(ldiv);  
    
    function refresh()
    {
        layerCount = graph.model.getChildCount(graph.model.root)
        listDiv.innerHTML = '';

        function addLayer(index, label, child, defaultParent)
        {
            var ldiv = document.createElement('div');
            ldiv.className = 'geToolbarContainer';

            ldiv.style.overflow = 'hidden';
            ldiv.style.position = 'relative';
            ldiv.style.padding = '4px';
            ldiv.style.height = '22px';
            ldiv.style.display = 'block';
            ldiv.style.backgroundColor = (Dialog.backdropColor == 'white') ? 'whiteSmoke' : Dialog.backdropColor;
            ldiv.style.borderWidth = '0px 0px 1px 0px';
            ldiv.style.borderColor = '#c3c3c3';
            ldiv.style.borderStyle = 'solid';
            ldiv.style.whiteSpace = 'nowrap';
            ldiv.setAttribute('title', label);
            
            var left = document.createElement('div');
            left.style.display = 'inline-block';
            left.style.width = '100%';
            left.style.textOverflow = 'ellipsis';
            left.style.overflow = 'hidden';
            
            mxEvent.addListener(ldiv, 'dragover', function(evt)
            {
                evt.dataTransfer.dropEffect = 'move';
                dropIndex = index;
                evt.stopPropagation();
                evt.preventDefault();
            });
            
            mxEvent.addListener(ldiv, 'dragstart', function(evt)
            {
                dragSource = ldiv;
                
                // Workaround for no DnD on DIV in FF
                if (mxClient.IS_FF)
                {
                    // LATER: Check what triggers a parse as XML on this in FF after drop
                    evt.dataTransfer.setData('Text', '<layer/>');
                }
            });
            
            mxEvent.addListener(ldiv, 'dragend', function(evt)
            {
                if (dragSource != null && dropIndex != null)
                {
                    graph.addCell(child, graph.model.root, dropIndex);
                }

                dragSource = null;
                dropIndex = null;
                evt.stopPropagation();
                evt.preventDefault();
            });

            var btn = document.createElement('img');
            btn.setAttribute('draggable', 'false');
            btn.setAttribute('align', 'top');
            btn.setAttribute('border', '0');
            btn.style.padding = '4px';
            btn.setAttribute('title', mxResources.get('lockUnlock'));

            var state = graph.view.getState(child);
                var style = (state != null) ? state.style : graph.getCellStyle(child);

            if (mxUtils.getValue(style, 'locked', '0') == '1')
            {
                btn.setAttribute('src', Dialog.prototype.lockedImage);
            }
            else
            {
                btn.setAttribute('src', Dialog.prototype.unlockedImage);
            }
            
            if (graph.isEnabled())
            {
                btn.style.cursor = 'pointer';
            }
            
            mxEvent.addListener(btn, 'click', function(evt)
            {
                if (graph.isEnabled())
                {
                    var value = null;
                    
                    graph.getModel().beginUpdate();
                    try
                    {
                        value = (mxUtils.getValue(style, 'locked', '0') == '1') ? null : '1';
                        graph.setCellStyles('locked', value, [child]);
                    }
                    finally
                    {
                        graph.getModel().endUpdate();
                    }

                    if (value == '1')
                    {
                        graph.removeSelectionCells(graph.getModel().getDescendants(child));
                    }
                    
                    mxEvent.consume(evt);
                }
            });

            left.appendChild(btn);

            var inp = document.createElement('input');
            inp.setAttribute('type', 'checkbox');
            inp.setAttribute('title', mxResources.get('hideIt', [child.value || mxResources.get('background')]));
            inp.style.marginLeft = '4px';
            inp.style.marginRight = '6px';
            inp.style.marginTop = '4px';
            left.appendChild(inp);
            
            if (graph.model.isVisible(child))
            {
                inp.setAttribute('checked', 'checked');
                inp.defaultChecked = true;
            }

            mxEvent.addListener(inp, 'click', function(evt)
            {
                graph.model.setVisible(child, !graph.model.isVisible(child));
                mxEvent.consume(evt);
            });

            mxUtils.write(left, label);
            ldiv.appendChild(left);
            
            if (graph.isEnabled())
            {
                // Fallback if no drag and drop is available
                if (mxClient.IS_TOUCH || mxClient.IS_POINTER || mxClient.IS_VML ||
                    (mxClient.IS_IE && document.documentMode < 10))
                {
                    var right = document.createElement('div');
                    right.style.display = 'block';
                    right.style.textAlign = 'right';
                    right.style.whiteSpace = 'nowrap';
                    right.style.position = 'absolute';
                    right.style.right = '6px';
                    right.style.top = '6px';
        
                    // Poor man's change layer order
                    if (index > 0)
                    {
                        var img2 = document.createElement('a');
                        
                        img2.setAttribute('title', mxResources.get('toBack'));
                        
                        img2.className = 'geButton';
                        img2.style.cssFloat = 'none';
                        img2.innerHTML = '&#9660;';
                        img2.style.width = '14px';
                        img2.style.height = '14px';
                        img2.style.fontSize = '14px';
                        img2.style.margin = '0px';
                        img2.style.marginTop = '-1px';
                        right.appendChild(img2);
                        
                        mxEvent.addListener(img2, 'click', function(evt)
                        {
                            if (graph.isEnabled())
                            {
                                graph.addCell(child, graph.model.root, index - 1);
                            }
                            
                            mxEvent.consume(evt);
                        });
                    }
        
                    if (index >= 0 && index < layerCount - 1)
                    {
                        var img1 = document.createElement('a');
                        
                        img1.setAttribute('title', mxResources.get('toFront'));
                        
                        img1.className = 'geButton';
                        img1.style.cssFloat = 'none';
                        img1.innerHTML = '&#9650;';
                        img1.style.width = '14px';
                        img1.style.height = '14px';
                        img1.style.fontSize = '14px';
                        img1.style.margin = '0px';
                        img1.style.marginTop = '-1px';
                        right.appendChild(img1);
                        
                        mxEvent.addListener(img1, 'click', function(evt)
                        {
                            if (graph.isEnabled())
                            {
                                graph.addCell(child, graph.model.root, index + 1);
                            }
                            
                            mxEvent.consume(evt);
                        });
                    }
                    
                    ldiv.appendChild(right);
                }
                
                if (mxClient.IS_SVG && (!mxClient.IS_IE || document.documentMode >= 10))
                {
                    ldiv.setAttribute('draggable', 'true');
                    ldiv.style.cursor = 'move';
                }
            }

            mxEvent.addListener(ldiv, 'dblclick', function(evt)
            {
                var nodeName = mxEvent.getSource(evt).nodeName;
                
                if (nodeName != 'INPUT' && nodeName != 'IMG')
                {
                    renameLayer(child);
                    mxEvent.consume(evt);
                }
            });

            if (graph.getDefaultParent() == child)
            {
                ldiv.style.background =  (Dialog.backdropColor == 'white') ? '#e6eff8' : '#505759';
                ldiv.style.fontWeight = (graph.isEnabled()) ? 'bold' : '';
                selectionLayer = child;
            }
            else
            {
                mxEvent.addListener(ldiv, 'click', function(evt)
                {
                    if (graph.isEnabled())
                    {
                        graph.setDefaultParent(defaultParent);
                        graph.view.setCurrentRoot(null);
                        refresh();
                    }
                });
            }
            
            listDiv.appendChild(ldiv);
        };
        
        // Cannot be moved or deleted
        for (var i = layerCount - 1; i >= 0; i--)
        {
            (mxUtils.bind(this, function(child)
            {
                addLayer(i, graph.convertValueToString(child) ||
                    mxResources.get('background'), child, child);
            }))(graph.model.getChildAt(graph.model.root, i));
        }
        
        var label = graph.convertValueToString(selectionLayer) || mxResources.get('background');
        removeLink.setAttribute('title', mxResources.get('removeIt', [label]));
        duplicateLink.setAttribute('title', mxResources.get('duplicateIt', [label]));
        dataLink.setAttribute('title', mxResources.get('editData'));

        if (graph.isSelectionEmpty())
        {
            insertLink.className = 'geButton mxDisabled';
        }
    };

    refresh();
    graph.model.addListener(mxEvent.CHANGE, function()
    {
        refresh();
    });

    graph.selectionModel.addListener(mxEvent.CHANGE, function()
    {
        if (graph.isSelectionEmpty())
        {
            insertLink.className = 'geButton mxDisabled';
        }
        else
        {
            insertLink.className = 'geButton';
        }
    });

    this.window = new mxWindow(mxResources.get('layers'), div, x, y, w, h, true, true);
    this.window.minimumSize = new mxRectangle(0, 0, 120, 120);
    this.window.destroyOnClose = false;
    this.window.setMaximizable(false);
    this.window.setResizable(true);
    this.window.setClosable(true);
    this.window.setVisible(true);

    this.window.addListener(mxEvent.SHOW, mxUtils.bind(this, function()
    {
        this.window.fit();
    }));
    
    // Make refresh available via instance
    this.refreshLayers = refresh;
    
    this.window.setLocation = function(x, y)
    {
        var iw = window.innerWidth || document.body.clientWidth || document.documentElement.clientWidth;
        var ih = window.innerHeight || document.body.clientHeight || document.documentElement.clientHeight;
        
        x = Math.max(0, Math.min(x, iw - this.table.clientWidth));
        y = Math.max(0, Math.min(y, ih - this.table.clientHeight - 48));

        if (this.getX() != x || this.getY() != y)
        {
            mxWindow.prototype.setLocation.apply(this, arguments);
        }
    };
    
    var resizeListener = mxUtils.bind(this, function()
    {
        var x = this.window.getX();
        var y = this.window.getY();
        
        this.window.setLocation(x, y);
    });
    
    mxEvent.addListener(window, 'resize', resizeListener);

    this.destroy = function()
    {
        mxEvent.removeListener(window, 'resize', resizeListener);
        this.window.destroy();
    }
};

/**
 * Constructs a new edit file dialog.
 */
var EditMiniConfigDialog = function(editorUi,vertices,edges)
{
    var div = document.createElement('div');
    div.style.textAlign = 'right';

    var header = document.createElement('h2');
    header.textContent = "Minimega Script";
        header.style.marginTop = "0";
        header.style.marginBottom = "10px";
    header.style.textAlign = 'left';

        div.appendChild(header);

    var textarea = document.createElement('textarea');
    textarea.setAttribute('wrap', 'off');
    textarea.setAttribute('spellcheck', 'false');
    textarea.setAttribute('autocorrect', 'off');
    textarea.setAttribute('autocomplete', 'off');
    textarea.setAttribute('autocapitalize', 'off');
    textarea.style.overflow = 'auto';
    textarea.style.resize = 'none';
    textarea.style.width = '100%';
    textarea.style.height = '360px';
    textarea.style.lineHeight = 'initial';
    textarea.style.marginBottom = '16px';
    var vlans_in_use = {};
    var vlan_count =10;
    var vlanid = "a";
    function searchNextVlan(v){
        if (!vlans_in_use.hasOwnProperty(v.toString())){
            vlans_in_use[v.toString()]=true;
            return v;
        }
        while (vlans_in_use.hasOwnProperty(vlanid)){
            c = vlanid.charCodeAt(vlanid.length-1);
            if (c == 122){
                index = v.length-1;
                carry = 0;
                while (index > -1){
                    if (vlanid.charCodeAt(index) == 122){
                        vlanid = vlanid.substr(0, index) + "a"+ vlanid.substr(index + 1);
                        carry++;
                    }
                    else {
                        vlanid = vlanid.substr(0, index) + String.fromCharCode(vlanid.charCodeAt(index) + 1)+ vlanid.substr(index + 1);
                    }
                    index--;
                }
                if (carry == vlanid.length){
                    vlanid += "a";
                }
            }
            else {
                vlanid = vlanid.substr(0, vlanid.length -1 ) + String.fromCharCode(c + 1);
            }
        }
        vlans_in_use[vlanid]=true;
        return vlanid;
    }

    // Standardizes all cells to ahve standard value object
    function checkValue(cell){
        var value = cell.getValue();
        if (!mxUtils.isNode(value)){
            var doc = mxUtils.createXmlDocument();
            var obj = doc.createElement('object');
            obj.setAttribute('label', value || '');
            value = obj;
            cell.setValue(value);
        }
    }

    function lookforvlan(cell){
        checkValue(cell);
        // Check if vertex is a switch, if it is and it does not have a vlan set all edges to a new vlan
        if (cell.getStyle().includes("switch")){
            if (cell.getAttribute("vlan") == undefined ){
                cell.setAttribute("vlan", searchNextVlan(vlan_count).toString());
            } 
            for (var i =0; i< cell.getEdgeCount();i++){
                var e = cell.getEdgeAt(i);
                checkValue(e);
                e.setAttribute("vlan",cell.getAttribute("vlan"));
            }
        } else {
            for (var i =0; i< cell.getEdgeCount();i++){
                var e = cell.getEdgeAt(i);
                checkValue(e);
                // check if edge has a vlan
                if (e.getAttribute("vlan") != undefined) {
                    if (!vlans_in_use.hasOwnProperty(e.getAttribute("vlan"))){
                        vlans_in_use[e.getAttribute("vlan")]=true;
                    }
                    continue
                }
                var ec;

                // Figure out which end is the true target for the edge
                if (e.source.getId() != cell.getId()){
                    ec = e.source;
                } else {ec = e.target;}
                if (ec == null){
                    return;
                }
                checkValue(ec);

                // if connected vertex is a switch get the vlan number or sets one for the switch and the edge
                if (ec.getStyle().includes("switch")){
                    if (ec.getAttribute("vlan") == undefined){
                        e.setAttribute("vlan", searchNextVlan(vlan_count).toString());
                        ec.setAttribute("vlan", e.getAttribute("vlan"));
                    } else {e.setAttribute("vlan", ec.getAttribute("vlan"));}
                } // If its any other device just set a new vlan to the edge
                else {
                    e.setAttribute("vlan", searchNextVlan(vlanid).toString());
                }
            }
        }
    }

    //Walk through all existing edges
    
    edges.forEach(e => {
    checkValue(e);
    if (e.getAttribute("vlan") != undefined){
        if (!vlans_in_use.hasOwnProperty(e.getAttribute("vlan"))){
            vlans_in_use[e.getAttribute("vlan")]=true;
        }
    }});
    
    var count =0;
    var parameters = {memory:"2048", vcpu:"1", network:undefined,kernel:undefined,initrd:undefined,disk:undefined,snapshot:true,cdrom:undefined};
    var config = "";
    var prev_dev_config = "";
    var prev_dev = {};
    vertices.forEach(cell => {
        var dev_config="";
        var name = "";
        lookforvlan(cell);
        // if vertex is a switch skip the device in config
        cell.setAttribute("type","diagraming");
        if (cell.getStyle().includes("switch")){return;}
        if (cell.getStyle().includes("router")){cell.setAttribute("type","router");}
        if (cell.getStyle().includes("firewall")){cell.setAttribute("type","firewall");}
        if (cell.getStyle().includes("desktop")){cell.setAttribute("type","desktop");}
        if (cell.getStyle().includes("server")){cell.setAttribute("type","server");}
        if (cell.getStyle().includes("mobile")){cell.setAttribute("type","mobile");}
        if (cell.getAttribute("type") == "diagraming"){
            return;
        }
        
        if (cell.getAttribute("name") != undefined) {
            config += `## Config for ${cell.getAttribute("name")}\n`;
            name = cell.getAttribute("name");
        } 
        else {
            config+= `##Config for a ${cell.getAttribute("type")} device #${count}\n`;
            name = `${cell.getAttribute("type")}_device_${count}`
        }
        count++;

        var clear ="";
        var net ="";
                for (var i =0; i< cell.getEdgeCount();i++){
                    var e = cell.getEdgeAt(i);
                    net += `vlan-${e.getAttribute("vlan")}`;
                    if (i+1 < cell.getEdgeCount()){
                        net += ' ';
                    }
                }
                if (net == ""){
                    delete prev_dev["network"];
                    clear += "clear vm config network"
                }
                else {
                    if (cell.getAttribute("network") != net){
                        cell.setAttribute("network",net);
                    }
                    if (prev_dev["network"] != net){
                    prev_dev["network"] = net
                    config += `vm config network ${net} \n`;
                    }
                }

        // Generate configuration for parameters
        for (const p in parameters) {
            // If there is no configuration for a parameter and the previous device had one clear it
            if ((cell.getAttribute(p) == undefined || cell.getAttribute(p) == "undefined") && prev_dev.hasOwnProperty(p)){
                if (p != "network"){
                    delete prev_dev[p];
                    clear +=`clear vm config ${p}\n`;
                }
            }
            // if it has a configuration for the parameter set it else issue a default value
            if (cell.getAttribute(p) != undefined && cell.getAttribute(p) != "undefined") { 
                if(prev_dev[p] != cell.getAttribute(p)){
                    prev_dev[p] = cell.getAttribute(p);
                    config += `vm config ${p} ${cell.getAttribute(p)} \n`;
                }
            }
            else {
                //if (parameters[p] != undefined && !prev_dev_config.includes(`vm config ${p} ${parameters[p]} \n`)){
                if (parameters[p] != undefined && prev_dev[p] != parameters[p]){
                    //dev_config += `vm config ${p} ${parameters[p]} \n`;
                    prev_dev[p] = cell.getAttribute(p);
                    config += `vm config ${p} ${parameters[p]} \n`;
                }
            }
          }
        
        config += clear;
        if (cell.getStyle().includes("container")){config+=`vm launch container ${name}\n\n`;}
        else {config+=`vm launch kvm ${name}\n\n`;}
    });
    textarea.value = config + "## Starting all VM's\nvm start all\n";
    div.appendChild(textarea);
    
    this.init = function()
    {
        textarea.focus();
    };
    
    // Enables dropping files
    if (Graph.fileSupport)
    {
        function handleDrop(evt)
        {
            evt.stopPropagation();
            evt.preventDefault();
            
            if (evt.dataTransfer.files.length > 0)
            {
                var file = evt.dataTransfer.files[0];
                var reader = new FileReader();
                
                reader.onload = function(e)
                {
                    textarea.value = e.target.result;
                };
                
                reader.readAsText(file);
            }
            else
            {
                textarea.value = editorUi.extractGraphModelFromEvent(evt);
            }
        };
        
        function handleDragOver(evt)
        {
            evt.stopPropagation();
            evt.preventDefault();
        };

        // Setup the dnd listeners.
        textarea.addEventListener('dragover', handleDragOver, false);
        textarea.addEventListener('drop', handleDrop, false);
    }

        var formdiv = document.createElement('div');
        div.appendChild(formdiv);

        var cbdiv = document.createElement('div');
        cbdiv.style.float = "left";
        formdiv.appendChild(cbdiv);

        var checkbox = document.createElement('input');
        checkbox.type = "checkbox";

        var checkboxLabel = document.createElement('label');
        checkboxLabel.textContent = 'Clear previous experiment?';

        cbdiv.appendChild(checkbox);
        cbdiv.appendChild(checkboxLabel);
    
    var cancelBtn = mxUtils.button(mxResources.get('cancel'), function()
    {
        editorUi.hideDialog();
    });
    cancelBtn.className = 'geBtn';
    
    if (editorUi.editor.cancelFirst)
    {
        formdiv.appendChild(cancelBtn);
    }

    var runBtn = mxUtils.button(mxResources.get('run'), function()
    {
        // Removes all illegal control characters before parsing
        var data = Graph.zapGremlins(mxUtils.trim(textarea.value));

        let cmds = data.split('\n').map(l => {
          return l.trim();
        }).filter(l => {
          return !(l === '' || l.startsWith('#'));
        }).map(l => {
          return {
            command: l
          };
        });

                var resetmm = [{command: "clear all"}];
                if (checkbox.checked) {
                  var tmp = [];
                  tmp.push(...resetmm);
                  tmp.push(...cmds);
                  cmds = tmp;
                }

        var responseDlg = new MiniResponseDialog(editorUi);
        $.post('/commands', JSON.stringify(cmds), function(resp){
          editorUi.showDialog(responseDlg.container, 820, 600, true, false);
          responseDlg.init();
          for (let i = 0; i < resp.length; i++) {
            let rs  = resp[i];
            let cmd  = cmds[i];
            responseDlg.appendRow(cmd.command, rs);
          }
        }, "json");

        editorUi.hideDialog();
    });
    runBtn.className = 'geBtn gePrimaryBtn';
    formdiv.appendChild(runBtn);

    if (!editorUi.editor.cancelFirst)
    {
        formdiv.appendChild(cancelBtn);
    }

    this.container = div;
};

/**
 *
 */
EditMiniConfigDialog.showNewWindowOption = true;

var MiniResponseDialog = function(editorUi)
{
    var div = document.createElement('div');

    var header = document.createElement('h2');
    header.textContent = "Minimega Response";
        header.style.marginTop = "0";
        header.style.marginBottom = "10px";

    var tdiv = document.createElement('div');
    tdiv.style.overflowY = 'scroll';
    tdiv.style.height = '500px';

    var table = document.createElement('table');
    table.setAttribute('wrap', 'off');
    table.setAttribute('spellcheck', 'false');
    table.setAttribute('autocorrect', 'off');
    table.setAttribute('autocomplete', 'off');
    table.setAttribute('autocapitalize', 'off');
    table.style.overflow = 'auto';
    table.style.resize = 'none';
    table.style.width = '800px';
    table.style.height = '100%';
    table.style.lineHeight = 'initial';
    table.style.marginBottom = '16px';

        tdiv.appendChild(table);

    div.appendChild(header);
    div.appendChild(tdiv);

    let theader = document.createElement('tr');
    theader.style.verticalAlign = 'top';
    theader.style.fontWeight = 'bold';

    let cmdTh = document.createElement('th');
    cmdTh.textContent = "Command";
    theader.appendChild(cmdTh);

    let respTh = document.createElement('th');
    respTh.textContent = "Response";
    theader.appendChild(respTh);

    table.appendChild(theader);

    this.appendRow = function(cmd, resps) {
      let row = document.createElement('tr');
      row.style.verticalAlign = 'top';

      let cmdTd = document.createElement('td');
      let respTd = document.createElement('td');

      cmdTd.textContent = cmd;

      for (let r of resps){
        if (r.Error) {
          respTd.textContent = "Error: " + r.Error;
          respTd.style.color = "red";
          respTd.style.fontWeight = "bold";
        } else if (r.Response) {
          respTd.textContent = r.Response;
        } else {
              // blank response
              respTd.innerHTML = "&#10004;"
              respTd.style.color = "green";
            }
      }

      table.appendChild(row);
      row.appendChild(cmdTd);
      row.appendChild(respTd);
    }

    this.init = function()
    {
        table.focus();
    };

        var formdiv = document.createElement('div');
        formdiv.style.textAlign = 'right';
        formdiv.style.marginTop = '20px';
        div.appendChild(formdiv);

    var cancelBtn = mxUtils.button(mxResources.get('cancel'), function()
    {
        editorUi.hideDialog();
    });
    cancelBtn.className = 'geBtn';

    if (editorUi.editor.cancelFirst)
    {
        formdiv.appendChild(cancelBtn);
    }

    var okBtn = mxUtils.button(mxResources.get('ok'), function()
    {
        editorUi.hideDialog();
    });
    okBtn.className = 'geBtn gePrimaryBtn';
    formdiv.appendChild(okBtn);
    
    if (!editorUi.editor.cancelFirst)
    {
        formdiv.appendChild(cancelBtn);
    }

    this.container = div;
};

/**
 * 
 */
MiniResponseDialog.showNewWindowOption = true;
