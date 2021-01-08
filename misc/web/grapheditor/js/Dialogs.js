/**
 * Copyright (c) 2006-2012, JGraph Ltd
 * Copyright 2017-2021 National Technology & Engineering Solutions of Sandia, LLC (NTESS). 
 * Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain 
 * rights in this software.
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
 * UTILITY FUNCTIONS below perform various data-manipulation operations
 * across topo-specific dialogs/actions (e.g., EditDataDialog, VariablesDialog, and
 * EditMiniConfigDialog, ViewJSONDialog)
 */

// utility function globals
const vlans_in_use = {};
const interfaces_in_use = {};
const vlan_count = 10;
let vlanid = "a";
let host_count = 0;

// utility function to handle non-vlan (diagraming)
function checkEdgeDiagraming(graph, edge) {
    var diagraming = false;
    var source = edge.source;
    var target = edge.target;
    if (!source && !target) {
        diagraming = true;
    }
    if (source){
        if (!source.hasAttribute('schemaVars')){
            diagraming = true;
        }
    }
    if (target) {
        if (!target.hasAttribute('schemaVars')){
            diagraming = true;
        }
    }
    // remove vlan attributes if edge now connected to diagraming node
    if (diagraming && mxUtils.isNode(graph.getModel().getValue(edge))) {
        if (edge.hasAttribute('schemaVars')) {
            var vlan = JSON.parse(edge.getAttribute('schemaVars')).name;
            if (source){
                if (source.hasAttribute('schemaVars')){
                    try {
                        removeNodeVlans(graph, source, vlan);
                    }
                    catch (e) {
                        // console.log(e);
                    }
                }
            }
            if (target) {
                if (target.hasAttribute('schemaVars')){
                    try {
                        removeNodeVlans(graph, target, vlan);
                    }
                    catch {

                    }
                }
            }
            var doc = mxUtils.createXmlDocument();
            var obj = doc.createElement('object');
            obj.setAttribute('label', '');
            graph.getModel().setValue(edge, obj);
        }
    }
    return diagraming;
}

// utility function to remove disconnected/diagraming vlans from nodes
function removeNodeVlans(graph, cell, vlan) {
    var value = graph.getModel().getValue(cell); // value user object
    value = value.cloneNode(true); // clone for value update
    var schemaVars = JSON.parse(cell.getAttribute('schemaVars'));
    schemaVars.network.interfaces = schemaVars.network.interfaces.filter(function( obj ) {
        return obj.vlan !== vlan;
    });
    value.setAttribute('schemaVars', JSON.stringify(schemaVars));
    graph.getModel().setValue(cell, value);
}

// utility function to set cell defaults
// Standardizes all cells to have standard value object
// and instatiates schemaVars object
function checkValue(graph, cell, ui) {

    var value = graph.getModel().getValue(cell);
    if (!mxUtils.isNode(value))
    {
        var doc = mxUtils.createXmlDocument();
        var obj = doc.createElement('object');
        obj.setAttribute('label', value || '');
        value = obj;
    }

    value = value.cloneNode(true);

    var schemaVars; // stores schema-specific JSON data
    var device = 'diagraming';
    try {
        if (cell.getStyle().includes("switch")){device = "switch";}
        if (cell.getStyle().includes("router")){device = "router";}
        if (cell.getStyle().includes("firewall")){device = "firewall";}
        if (cell.getStyle().includes("desktop")){device = "desktop";}
        if (cell.getStyle().includes("server")){device = "server";}
        if (cell.getStyle().includes("mobile")){device = "mobile";}
    }
    catch {
        // console.log('no style');
    }

    if (value.hasAttribute('schemaVars')) {
        // update hostname if it's a duplicate and a vertex (for copied cells)
        if (cell.isVertex() && device != 'switch') {
            schemaVars = JSON.parse(value.getAttribute('schemaVars'));
            var hostname = cell.getAttribute('label');
            schemaVars.general.hostname = hostname;
            var cellId = parseInt(cell.getId());
            var filter = function(cell) {return graph.model.isVertex(cell);}
            var vertices = graph.model.filterDescendants(filter);
            for (var i = 0; i < vertices.length; i++) {
                var checkLabel = vertices[i].getAttribute('label');
                var checkHostId = parseInt(vertices[i].getId());
                if (checkLabel == hostname && checkHostId != cellId && cellId > checkHostId) {
                    schemaVars.general.hostname = `${schemaVars.device}_device_${host_count}`;
                    // hack to prevent ifaces/vlans from piling up in cloned node
                    try {
                        delete schemaVars.network;
                    }
                    catch {
                        // console.log('network object does not exist');
                    }
                    value.setAttribute('label', `${schemaVars.device}_device_${host_count}`);
                    host_count++;
                }
            }
            value.setAttribute('schemaVars', JSON.stringify(schemaVars));
            graph.getModel().setValue(cell, value);
            // remove vlans if cell doesn't have edges
            try {
                if (cell.getEdgeCount() == 0 && schemaVars.network.interfaces.length > 0) {
                    // delete schemaVars.network;
                }
            }
            catch {

            }
        }
        else if (cell.isEdge()) {
            checkEdgeDiagraming(graph, cell);
            return;
        }
    }
    else {

        schemaVars = {};

        if (cell.isVertex() && device != 'switch') {

            if (device == 'diagraming') {
                graph.getModel().setValue(cell, value);
                return;
            }

            let type;
            if (cell.getStyle().includes("container"))
            {
                type = 'container';
            }
            else
            {
                type = 'kvm';
            }

            const schema = ui.schemas[type]; // get schema to load default values

            let config = {
                schema: schema,
                startval: {},
                ajax: true,
                mode: 'text',
                modes: ['code', 'text', 'tree'],
                theme: 'bootstrap3',
                iconlib: 'spectre',
            };

            // hack to parse default schema values
            var element = document.createElement("div");
            element.setAttribute('id', 'jsoneditor');
            const jsoneditor = new JSONEditor(element, config);
            schemaVars = jsoneditor.getEditor('root').value; 

            schemaVars.type = type;
            schemaVars.device = device;

            if (typeof schemaVars.general === 'undefined') schemaVars.general = {};
            schemaVars.general.hostname = `${schemaVars.device}_device_${host_count}`; // (cell.getId());
            value.setAttribute('label', `${schemaVars.device}_device_${host_count}`);
            host_count++;

            if (typeof schemaVars.hardware === 'undefined') schemaVars.hardware = {};
            schemaVars.hardware.os_type = 'linux';

        }
        else {
            // only set default vlan value if device is switch
            schemaVars.id = 'auto';
            if (device == 'switch') {
                schemaVars.device = 'switch';
                schemaVars.hostname = `${schemaVars.device}_device_${host_count}`;
                value.setAttribute('label', `${schemaVars.device}_device_${host_count}`);
                schemaVars.name = searchNextVlan(vlanid).toString();
                host_count++;
            }
            else {
                if (checkEdgeDiagraming(graph, cell)) {
                    graph.getModel().setValue(cell, value);
                    return;
                }
                schemaVars.name = ''; // initial (edge) vlan name set when connected to cell
            }
        }

        value.setAttribute('schemaVars', JSON.stringify(schemaVars));
        graph.getModel().setValue(cell, value);

    }

}

// utility function to get/set next vlan name
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

// utility function to set node and edge vlan values
function lookforvlan(graph, cell, toSwitch=null){

    // ignore diagraming cells
    if (!cell.hasAttribute('schemaVars')) {
        return;
    }

    // checkValue(graph, cell);
    var value = graph.getModel().getValue(cell); // value user object
    value = value.cloneNode(true); // clone for value update
    var schemaVars = JSON.parse(cell.getAttribute('schemaVars')); // current cell (node) schemaVars
    var edgeSchemaVars; // edge schemaVars
    var targetSchemaVars; // target cell schemaVars
    var disconnectedVlans = []; // catch indices of bridge/disconnected iface vlans (e.g., manually added ifaces)

    try {
        schemaVars.network.interfaces;
        var vlans = []; // holds true edges
        var edges = cell.edges; // cell edges
        if (edges) {
            for(var i = 0; i < edges.length; i++){
                // checkValue(graph, edges[i]);
                if (checkEdgeDiagraming(graph, edges[i])) {
                    if (edges[i].hasAttribute('schemaVars')) {
                        var vlan = JSON.parse(edges[i].getAttribute('schemaVars')).name;
                        schemaVars.network.interfaces = schemaVars.network.interfaces.filter(function( obj ) {
                            return obj.vlan !== vlan;
                        });
                    }
                    continue;
                }
                var vlan = JSON.parse(edges[i].getAttribute('schemaVars')).name;
                if (vlan != '') vlans.push(vlan);
            }
        }
        // remove interfaces copied over from another cell
        // track indices of bridge/disconnected vlans so they don't get overwritten
        // console.log(toSwitch);
        for (var i = 0; i < schemaVars.network.interfaces.length; i++){
            // toSwitch parameter used to detect existing node's iface vlan edge connected to new switch
            // persists previous vlan name to ensure correct iface vlan is updated
            if (vlans.indexOf(schemaVars.network.interfaces[i].vlan) < 0 && schemaVars.network.interfaces[i].vlan != '' && schemaVars.network.interfaces[i].vlan != toSwitch) {
                disconnectedVlans.push({idx:i, vlan:schemaVars.network.interfaces[i].vlan});
                if (!vlans_in_use.hasOwnProperty(schemaVars.network.interfaces[i].vlan)){
                    vlans_in_use[schemaVars.network.interfaces[i].vlan]=true;
                }
            }
        }
        // console.log(JSON.stringify(schemaVars.network.interfaces));
        // if the edge is new and doesn't have a vlan name yet, vlans will be empty, so assign empty obj
        if (typeof schemaVars.network.interfaces[0] === 'undefined') schemaVars.network.interfaces[0] = {};
    }
    catch(e) {
        // console.log(e);
        if (schemaVars.device != 'switch') {
            schemaVars.network = {};
            schemaVars.network.interfaces = [];
            schemaVars.network.interfaces[0] = {};
        }
    }

    // Check if vertex is a switch, if it is and it does not have a vlan set all edges to a new vlan
    if (schemaVars.device == 'switch'){
        if (typeof schemaVars.name === 'undefined' || schemaVars.name == '') {
            schemaVars.name = searchNextVlan(vlanid).toString();
            schemaVars.id = 'auto';
            value.setAttribute('schemaVars', JSON.stringify(schemaVars));
        }
        for (var i =0; i < cell.getEdgeCount();i++){
            var e = cell.getEdgeAt(i);
            // skip diagraming edges
            if (checkEdgeDiagraming(graph, e)) {
                continue;
            }
            // checkValue(graph, e);
            var edgevalue = graph.getModel().getValue(e);
            edgevalue = edgevalue.cloneNode(true);
            edgeSchemaVars = JSON.parse(e.getAttribute('schemaVars'));
            var prevName = edgeSchemaVars.name;
            // console.log(prevName);
            edgeSchemaVars.name = schemaVars.name;
            // console.log(edgeSchemaVars.name);
            edgeSchemaVars.id = schemaVars.id;
            edgevalue.setAttribute('schemaVars', JSON.stringify(edgeSchemaVars));
            edgevalue.setAttribute('label', schemaVars.name);
            graph.getModel().setValue(e, edgevalue);

            try {
                if (e.source.getId() != cell.getId()){
                    ec = e.source;
                } else {ec = e.target;}
            }
            catch (e) {

            }
            if (ec) {
                // skip diagraming nodes
                if (!ec.hasAttribute('schemaVars')) {
                    continue;
                }
                // prevent infinite loop
                if (JSON.parse(ec.getAttribute('schemaVars')).device != 'switch') {
                    lookforvlan(graph, ec, prevName);
                }
            }
        }
        return;
    } 
    else {

        // check if cell (node) is replacing a switch by searching for duplicate vlans
        if (cell.getEdgeCount() > 1){
            var dupe = false;
            var name;
            for(var i = 0; i < cell.getEdgeCount(); i++) {
                var e = cell.getEdgeAt(i);
                if (checkEdgeDiagraming(graph, e)) {
                    continue;
                }
                // checkValue(graph, e);
                if (JSON.parse(e.getAttribute('schemaVars')).name == name && name != '') {
                    dupe = true;
                    name = JSON.parse(e.getAttribute('schemaVars')).name;
                    break;
                }
                else {
                    name = JSON.parse(e.getAttribute('schemaVars')).name;
                }
            }
        }
        // console.log(disconnectedVlans);
        var connectedVlanIdx = 0; // true idx of interface; accounts for bridges/vlans with no edges
        for (var i = 0; i < cell.getEdgeCount(); i++){
            for (var j = 0; j < disconnectedVlans.length; j++) {
                // if indices match AND cell has more than one edge*
                // *to account for existing vlan connected to switch
                if (connectedVlanIdx == disconnectedVlans[j].idx) {
                    connectedVlanIdx++;
                }
                else{
                    break;
                }
            }
            // console.log(connectedVlanIdx);
            var eth = 'eth' + connectedVlanIdx;
            var e = cell.getEdgeAt(i);

            // if edge is a diagraming edge, continue
            if (checkEdgeDiagraming(graph, e)) {
                continue;
            }
            // if edge is disconnected, continue
            if (!e.source || !e.target) {
                continue;
            }

            var edgevalue = graph.getModel().getValue(e);
            edgevalue = edgevalue.cloneNode(true);
            edgeSchemaVars = JSON.parse(e.getAttribute('schemaVars'));
            // if cell has edges with duplicate vlans, reset all vlans and reassign thereafter
            if (dupe) {
                edgeSchemaVars.name = '';
                edgeSchemaVars.id = 'auto';
            }

            var ec; // edge true target cell

            // Figure out which end is the true target for the edge
            try {
                if (e.source.getId() != cell.getId()){
                    ec = e.source;
                } else {ec = e.target;}
            }
            catch (e) {

            }

            if (ec && ec.hasAttribute('schemaVars')) {
                // checkValue(graph, ec);
                var targetvalue = graph.getModel().getValue(ec);
                targetvalue = targetvalue.cloneNode(true);
                targetSchemaVars = JSON.parse(ec.getAttribute('schemaVars'));

                try {
                    targetSchemaVars.network.interfaces; 
                    targetEth = 'eth' + (Math.max.apply(Math, (targetSchemaVars.network.interfaces).map(function(o) { return (o.name).substr(-1); })) + 1);
                }
                catch(e) {
                    if (targetSchemaVars.device != 'switch') {
                        targetSchemaVars.network = {};
                        targetSchemaVars.network.interfaces = [];
                        targetSchemaVars.network.interfaces[0] = {};
                        targetEth = 'eth0';
                    }
                }

                // if connected vertex is a switch get the vlan number or sets one for the switch and the edge
                if (targetSchemaVars.device == 'switch'){
                    if (typeof targetSchemaVars.name === 'undefined' || targetSchemaVars.name == ''){
                        edgeSchemaVars.name = searchNextVlan(vlanid).toString();
                        edgeSchemaVars.id = 'auto';
                        targetSchemaVars.name = edgeSchemaVars.name;
                        targetSchemaVars.id = edgeSchemaVars.id;
                    } else {
                        edgeSchemaVars.name = targetSchemaVars.name;
                        edgeSchemaVars.id = targetSchemaVars.id;
                        edgevalue.setAttribute('schemaVars', JSON.stringify(edgeSchemaVars));
                    }
                    targetvalue.setAttribute('schemaVars', JSON.stringify(targetSchemaVars));
                    graph.getModel().setValue(ec, targetvalue);
                } // if its any other device just set a new vlan to the edge
                else {
                    // set edge vlan and vlan id
                    if (typeof edgeSchemaVars.name === 'undefined' || edgeSchemaVars.name == '') {
                        edgeSchemaVars.name = searchNextVlan(vlanid).toString();
                        edgeSchemaVars.id = 'auto';
                        edgevalue.setAttribute('schemaVars', JSON.stringify(edgeSchemaVars));
                    }
                    var newVlan = true;
                    var matchIdx = -1;
                    for (var j = 0; j < targetSchemaVars.network.interfaces.length; j++){
                        try {
                            matchIdx++;
                            var findMatch;
                            if (dupe) {
                                findMatch = name;
                            }
                            else {
                                findMatch = edgeSchemaVars.name;
                            }
                            if (findMatch == targetSchemaVars.network.interfaces[j].vlan) {
                                newVlan = false;
                                break;
                            }   
                        }
                        catch {
                            
                        }
                    }
                    // set device vlan to edge vlan on a new interface if doesn't exist
                    if (newVlan) {
                        var newEth = 'eth' + matchIdx;
                        targetSchemaVars.network.interfaces[matchIdx] = {};
                        targetSchemaVars.network.interfaces[matchIdx].vlan = edgeSchemaVars.name;
                        targetSchemaVars.network.interfaces[matchIdx].name = newEth;
                    }
                    // set device vlan to edge vlan if interface exists
                    else {
                        targetSchemaVars.network.interfaces[matchIdx].vlan = edgeSchemaVars.name;
                    }   
                    targetvalue.setAttribute('schemaVars', JSON.stringify(targetSchemaVars));
                    graph.getModel().setValue(ec, targetvalue);
                }
            }

            if (typeof schemaVars.network.interfaces[connectedVlanIdx] === 'undefined') schemaVars.network.interfaces[connectedVlanIdx] = {};
            schemaVars.network.interfaces[connectedVlanIdx].vlan = edgeSchemaVars.name;
            schemaVars.network.interfaces[connectedVlanIdx].name = eth;
            connectedVlanIdx++;
            value.setAttribute('schemaVars', JSON.stringify(schemaVars));
            graph.getModel().setValue(cell, value);

            if (typeof edgeSchemaVars.name !== 'undefined' && edgeSchemaVars.name != '') {
                if (!vlans_in_use.hasOwnProperty(edgeSchemaVars.name)){
                    vlans_in_use[edgeSchemaVars.name]=true;
                }
                edgevalue.setAttribute('label', edgeSchemaVars.name);
                graph.getModel().setValue(e, edgevalue);
            }

        }
    }
}

/**
 * END UTILITY FUNCTIONS
 */


/**
 * Constructs a new JSONEditor for cell JSON
 */
var EditDataDialog = function(ui, cell)
{
    const graph = ui.editor.graph;
    
    // checkValue(graph, cell); // sets cell default user object values (for cloned cells)
    var value = graph.getModel().getValue(cell);
    let startval = {};
    let type;
    try {
        startval = JSON.parse(value.getAttribute('schemaVars')); // parse schemaVars to JSON for editor
        if (cell.isVertex()) {
            if (startval.device == 'switch') {
                type = 'switch';
            }
            else {
                type = startval.type;
            }
        }
        else {
            type = 'vlan';
        }
    }
    catch {
    
    }

    // check if cell is diagraming
    if (!cell.hasAttribute('schemaVars')) {
        type = 'diagraming';
        var label = cell.getAttribute('label') || '';
        startval = {text: label};
    }

    let schema = ui.schemas[type]; // set schema
    // console.log('this is schema'), console.log(schema);

    var id = (EditDataDialog.getDisplayIdForCell != null) ?
        EditDataDialog.getDisplayIdForCell(ui, cell) : null;
                    
    // Set JSONEditor and config options based on schema and cell type
    var loadConfig = function () {

        this.config = {
            schema: schema,
            startval: startval,
            ajax: true,
            mode: 'tree',
            modes: ['code', 'text', 'tree'],
            show_errors: 'always',
            theme: 'bootstrap3',
            iconlib: 'spectre',
            no_additional_properties: true,
            disable_edit_json: true,
            display_required_only: true
        };

        createEditor();
    };

    var createEditor = function() {

        var div = document.createElement('div');
        div.style['overflow-y'] = 'auto';

        var editorContainer = document.createElement('div');
        editorContainer.setAttribute('id', 'jsoneditor');
        editorContainer.style.height = '100%';
        editorContainer.style.position = 'relative';
        editorContainer.style['max-width'] ="600px";

        const jsoneditor = new JSONEditor(editorContainer, this.config);

        var filter = function(cell) {return graph.model.isEdge(cell);}
        var edges = graph.model.filterDescendants(filter);

        // disable vlan attribute if cell is a node and not a switch;
        // disable/enable various properties depending on type
        // edge values dictate node vlan attribute values unless it's a switch
        function disableFields() {
            if (cell.isVertex() && cell.hasAttribute('schemaVars')) {
                if (jsoneditor.getEditor('root.device').getValue() != 'switch') {
                    for (var i = 0; i < jsoneditor.getEditor('root.network.interfaces').getValue().length; i++) {
                        for (var j = 0; j < edges.length; j++) {
                            if(edges[j].hasAttribute('schemaVars')) {
                                var edgeSchemaVars = JSON.parse(edges[j].getAttribute('schemaVars'));
                                if (jsoneditor.getEditor('root.network.interfaces.'+i+'.vlan').getValue() == edgeSchemaVars.name) {
                                    jsoneditor.getEditor('root.network.interfaces.'+i+'.vlan').disable();
                                }
                            }
                        }                
                    }
                }
            }
            else if (cell.isEdge()) {
                // disable attributes of edge if connected to a switch, since switch dictates values
                if (cell.hasAttribute('schemaVars')) {
                    if ( JSON.parse(cell.source.getAttribute('schemaVars')).device == 'switch' || JSON.parse(cell.target.getAttribute('schemaVars')).device == 'switch') {
                        jsoneditor.getEditor('root.name').disable();
                    }
                }
            }
        }

        try {
            disableFields();
        }
        catch (e) {
           // console.log(e);
        }

        div.appendChild(editorContainer);

        var cancelBtn = mxUtils.button(mxResources.get('cancel'), function()
        {
            ui.hideDialog.apply(ui, arguments);
            jsoneditor.destroy();
        });
        
        cancelBtn.className = 'geBtn';
        
        var applyBtn = mxUtils.button(mxResources.get('apply'), function()
        {
            const errors = jsoneditor.validate();
            var validForm = (errors.length) ? false : true;

            if (validForm) {
                try
                {
                    ui.hideDialog.apply(ui, arguments);
                    value = value.cloneNode(true);
                    var updatedNode = jsoneditor.getEditor('root').value; // get current node's JSON (from JSONEditor)
                    if (type != 'diagraming') {
                        value.setAttribute('schemaVars', JSON.stringify(updatedNode));
                        if (cell.isVertex()) {
                            if (updatedNode.device == 'switch') {
                                value.setAttribute('label', updatedNode.hostname);
                            }
                            else {
                                value.setAttribute('label', updatedNode.general.hostname);
                            }
                            graph.getModel().setValue(cell, value);
                            lookforvlan(graph, cell);
                        }
                        else if (cell.isEdge()) {
                            value.setAttribute('label', updatedNode.name);
                            var name = updatedNode.name;
                            var id = updatedNode.id;
                            // if an edge's id changes, apply change to all vlans with same name (e.g., connected to switch)
                            edges.forEach(cell => {
                                var edgevalue = graph.getModel().getValue(cell);
                                edgevalue = edgevalue.cloneNode(true);
                                var schemaVars = JSON.parse(edgevalue.getAttribute('schemaVars'));
                                if (name == schemaVars.name) {
                                    schemaVars.id = id;
                                    edgevalue.setAttribute('schemaVars', JSON.stringify(schemaVars));
                                    graph.getModel().setValue(cell, edgevalue);
                                }
                            });
                            graph.getModel().setValue(cell, value);
                            // TODO: call lookforvlan on source and target
                            var source = cell.source;
                            if (source) {
                                lookforvlan(graph, source);
                            }
                            var target = cell.target;
                            if (target) {
                                lookforvlan(graph, target);
                            }
                        }
                    }
                    else {
                        value.setAttribute('label', updatedNode.text);
                        graph.getModel().setValue(cell, value);
                    }
                }
                catch (e)
                {
                    mxUtils.alert(e);
                }

                jsoneditor.destroy();
            }
            else {
                mxUtils.alert(JSON.stringify(errors));
            }

        });

        applyBtn.className = 'geBtn gePrimaryBtn';

        jsoneditor.on('change',() => {
            const errors = jsoneditor.validate();
            if (errors.length) {applyBtn.setAttribute('disabled', 'disabled');}
            else {applyBtn.removeAttribute('disabled');}
            listenToDynamicElements();
        });
        
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

        // show dialog only after editor is created
        ui.showDialog(this.container, 480, 420, true, false, null, false); 

        // hack to resolve jsoneditor bug where disabled field is enabled when using properties dropdown
        // adds listeners to dynamically-created elements in jsoneditor window
        // https://github.com/jdorn/json-editor/pull/716
        function listenToDynamicElements() {
            [...document.querySelectorAll('#jsoneditor .json-editor-btn-edit_properties')].forEach(function(item) {
                if (item.getAttribute('listener') != 'click') {
                    item.addEventListener('click', function() {
                        item.setAttribute('listener', 'click');
                        try {
                            disableFields();
                        }
                        catch (e) {
                           // console.log(e);
                        }
                    });
                }
            });
            [...document.querySelectorAll('#jsoneditor select')].forEach(function(item) {
                if (item.getAttribute('listener') != 'change') {
                    item.addEventListener('change', function() {
                        item.setAttribute('listener', 'change');
                        try {
                            disableFields();
                        }
                        catch (e) {
                           // console.log(e);
                        }
                    });
                }
            });
        }
        listenToDynamicElements();

    };

    this.init = function()
    {
        loadConfig();
    };

};

/**
 * Constructs a new JSONEditor dialog for model JSON
 */
var viewJSONDialog = function(ui)
{
    const graph = ui.editor.graph;
   
    var filter = function(cell) {return graph.model.isVertex(cell);}
    var vertices = graph.model.filterDescendants(filter);
    filter = function(cell) {return graph.model.isEdge(cell);}
    var edges = graph.model.filterDescendants(filter);

    var nodeArray = [];
    var edgeArray = [];

    vertices.forEach(cell => {
        // checkValue(graph, cell);
        if (cell.hasAttribute('schemaVars')) {
            lookforvlan(graph, cell);
            var node = JSON.parse(cell.getAttribute('schemaVars'));
            if (node.device != 'switch') {
                nodeArray.push(node);
            }
        }
    });

    edges.forEach(cell => {
        // checkValue(graph, cell);
        if (cell.hasAttribute('schemaVars')) {
            var vlan = JSON.parse(cell.getAttribute('schemaVars'));
            if (edgeArray.filter(function(e) { return e.name == vlan.name; }).length <= 0 && vlan.id != 'auto') {
                edgeArray.push(vlan);
            }
        }
    });

    // hack combining node and edge schema programmatically
    // TODO: enforce comprehensive JSON schema for in-view editing
    const combinedSchema = JSON.parse('{"type": "object", "title": "Topo", "properties": { "nodes" : ' + JSON.stringify(ui.schemas['node']) + ', "vlans": ' + JSON.stringify(ui.schemas['vlan']) + '}}'); 
    // console.log(combinedSchema);

    // const schema = {}; // set schema
    let json = {nodes: nodeArray, vlans: edgeArray}; // global model JSON

    if (window.experiment_vars != undefined)
    {
        var jsonString = JSON.stringify(json);
        for (var i = 0; i < window.experiment_vars.length; i++)
        {
            var name = window.experiment_vars[i].name;
            var value = window.experiment_vars[i].value;

            var name = new RegExp('\\$'+name, 'g');
            jsonString = jsonString.replace(name, value);
        }
        json = JSON.parse(jsonString);
    }

    // Set JSONEditor and config options based on schema and cell type
    var loadConfig = function () {

        this.config = {
            schema: combinedSchema,
            startval: json,
            ajax: true,
            mode: 'text',
            modes: ['code', 'text', 'tree'],
            theme: 'bootstrap3',
            iconlib: 'spectre',
        };

        createEditor();
    };

    var createEditor = function() {

        var div = document.createElement('div');

        var header = document.createElement('h2');
        header.textContent = "Diagram JSON";
        header.style.marginTop = "0";
        header.style.marginBottom = "10px";
        header.style.textAlign = 'left';

        var editorContainer = document.createElement('div');
        editorContainer.setAttribute('id', 'jsoneditor');
        editorContainer.style.height = '100%';
        editorContainer.style.position = 'relative';
        // editorContainer.style['max-width'] ="600px";
        // editorContainer.style.display = 'none';

        var textarea = document.createElement('textarea');
        textarea.setAttribute('readonly', 'readonly');
        textarea.value = JSON.stringify(json, null, 2);
        textarea.setAttribute('id', 'jsonString');
        textarea.setAttribute('wrap', 'off');
        textarea.setAttribute('spellcheck', 'false');
        textarea.setAttribute('autocorrect', 'off');
        textarea.setAttribute('autocomplete', 'off');
        textarea.setAttribute('autocapitalize', 'off');
        textarea.style.overflow = 'auto';
        textarea.style.resize = 'none';
        textarea.style.width = '100%';
        textarea.style.height = '420px';
        textarea.style.lineHeight = 'initial';
        textarea.style.marginBottom = '16px';
        // textarea.setAttribute('onkeyup', 'autoHeight(this)');

        var textareaScript = document.createElement('script');
        var code = '/*var autoHeight = function(o) {o.style.height = "1px";o.style.height = (25+o.scrollHeight)+"px";}; autoHeight(document.getElementById("jsonString"));*/document.getElementById("copyJSON").onclick = function(){document.getElementById("jsonString").select();document.execCommand("copy");}';
        textareaScript.innerText = code;

        editorContainer.append(textareaScript);
        editorContainer.append(textarea);

        // TODO: enable json editing using editor and/or textarea
        // var validateTextarea = document.createElement('textarea');
        // validateTextarea.setAttribute('id', 'validate-textarea');
        // validateTextarea.setAttribute('disabled', 'disabled');
        // validateTextarea.setAttribute('readonly', 'readonly');
        // validateTextarea.style.width = '100%';
        // validateTextarea.style.resize = 'none';
        // validateTextarea.style.height = '80px';
        // validateTextarea.style.overflow = 'auto';

        // const jsoneditor = new JSONEditor(editorContainer, this.config);

        // listen for changes
        // jsoneditor.on('change', function () {
        //     // output
        //     let jsonval = jsoneditor.getValue();
        //     // textarea.value = JSON.stringify(jsonval, null, 2);

        //     // validate
        //     let validationErrors = jsoneditor.validate();
        //     console.log(validationErrors);
        //     if (validationErrors.length) {
        //         validateTextarea.value = JSON.stringify(validationErrors, null, 2);
        //     } else {
        //         validateTextarea.value = 'valid';
        //     }
        // });

        // textarea.addEventListener('keyup', function () {
        //     try {
        //         jsoneditor.setValue(JSON.parse(textarea.value));
        //     }
        //     catch (e) {
        //         console.log('catch'), console.log(e);
        //         validateTextarea.value = e;
        //     }
            
        // });

        div.appendChild(header);
        div.appendChild(editorContainer);
        // div.appendChild(validateTextarea);

        var cancelBtn = mxUtils.button(mxResources.get('cancel'), function()
        {
            ui.hideDialog.apply(ui, arguments);
            // jsoneditor.destroy();
        });
        
        cancelBtn.className = 'geBtn';
        cancelBtn.innerHTML = 'Close';
        
        var applyBtn = mxUtils.button(mxResources.get('apply'), function()
        {
            try
            {
                // console.log('do something here');

            }
            catch (e)
            {
                mxUtils.alert(e);
            }

            // jsoneditor.destroy();

        });

        applyBtn.className = 'geBtn gePrimaryBtn';
        applyBtn.innerHTML = 'Copy';
        applyBtn.setAttribute('id', 'copyJSON');

        var dlBtn = mxUtils.button(mxResources.get('apply'), function()
        {
            try
            {
                // console.log('download');
                downloadObjectAsJson(json, 'topo');

            }
            catch (e)
            {
                mxUtils.alert(e);
            }

            // jsoneditor.destroy();

        });

        // https://stackoverflow.com/questions/19721439/download-json-object-as-a-file-from-browser
        function downloadObjectAsJson(exportObj, exportName){
            var dataStr = "data:text/json;charset=utf-8," + encodeURIComponent(JSON.stringify(exportObj));
            var downloadAnchorNode = document.createElement('a');
            downloadAnchorNode.setAttribute("href",     dataStr);
            downloadAnchorNode.setAttribute("download", exportName + ".json");
            document.body.appendChild(downloadAnchorNode); // required for firefox
            downloadAnchorNode.click();
            downloadAnchorNode.remove();
        }

        dlBtn.className = 'geBtn gePrimaryBtn';
        dlBtn.innerHTML = 'Download';
        dlBtn.setAttribute('id', 'dowloadJSON');
        
        var buttons = document.createElement('div');
        buttons.style.cssText = 'position:absolute;left:30px;right:30px;text-align:right;bottom:15px;height:40px;padding-top:20px;margin-bottom:15px;'
        
        if (ui.editor.cancelFirst)
        {
            buttons.appendChild(cancelBtn);
            buttons.appendChild(applyBtn);
            buttons.appendChild(dlBtn);
        }
        else
        {
            buttons.appendChild(applyBtn);
            buttons.appendChild(dlBtn);
            buttons.appendChild(cancelBtn);
        }

        div.appendChild(buttons);
        this.container = div;

        // show dialog only after editor is created
        ui.showDialog(this.container, 480, 420, true, false, null, false); 
        progressDestroy();

    };

    this.init = function()
    {
        loadConfig();
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
 * Constructs a new JSONEditor for minimega config script
 */
var EditMiniConfigDialog = function(editorUi,vertices,edges)
{

        const graph = editorUi.editor.graph;

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
        textarea.style.height = '420px';
        textarea.style.lineHeight = 'initial';
        textarea.style.marginBottom = '16px';

        //Walk through all existing edges
        var edgeArray = [];
        edges.forEach(cell => {
            // checkValue(graph, cell);
            if (cell.hasAttribute('schemaVars')) {
                var vlan = JSON.parse(cell.getAttribute('schemaVars'));
                if (typeof vlan.name !== 'undefined' && vlan.name != '') {
                    if (!vlans_in_use.hasOwnProperty(vlan.name)){
                        vlans_in_use[vlan.name]=true;
                    }
                    if (edgeArray.filter(function(e) { return e.name == vlan.name; }).length <= 0 && vlan.id != 'auto') {
                        edgeArray.push(vlan);
                    }
                }
            }
        });

        var count = 0;
        var parameters = editorUi.params; // minimega parameter->schema maps for script generation

        // utility function to return path array
        const getPath = (path) => {
            var paths = path.split('.');
            var pathString;
            for (p in paths) {
                pathString += []
            }
            return paths;
        };

        // utility function to get minimega params from JSON for config
        // currently not used, but might be worth revisiting down the road
        // https://stackoverflow.com/questions/15523514/find-by-key-deep-in-a-nested-array
        // function getParams(object, key, result){
        //     if(object.hasOwnProperty(key))
        //         result.push(object[key]);

        //     for(var i=0; i<Object.keys(object).length; i++){
        //         if(typeof object[Object.keys(object)[i]] == "object"){
        //             getParams(object[Object.keys(object)[i]], key, result);
        //         }
        //     }
        // }

        var config = "";
        var prev_dev_config = "";
        var prev_dev = {};
        var schemaVars;
        var miniccc_commands = [];
        var networks = [];
        vertices.forEach(cell => {

            if (!cell.hasAttribute('schemaVars')) {
                return;
            }
            // checkValue(graph, cell);
            var value = graph.getModel().getValue(cell); // value user object
            value = value.cloneNode(true); // clone for value update
            schemaVars = JSON.parse(cell.getAttribute('schemaVars'));
            lookforvlan(graph, cell);

            // if vertex is a switch skip the device in config
            if (schemaVars.device == 'switch'){
                return;
            }

            var dev_config = "";
            var name = "";
            
            try {
                if (schemaVars.general.hostname != "" && typeof schemaVars.general.hostname != 'undefined') {
                    config += `## Config for ${schemaVars.general.hostname}\n`;
                    name = schemaVars.general.hostname;
                } 
                else{
                    config += `##Config for a ${schemaVars.device} device #${count}\n`;
                    name = `${schemaVars.device}_device_${count}`
                }
            }
            catch {
                config += `##Config for a ${schemaVars.device} device #${count}\n`;
                name = `${schemaVars.device}_device_${count}`
            }
            value.setAttribute('label', name);
            graph.getModel().setValue(cell, value);
            count++;

            if (schemaVars.network) {
                var netObj = schemaVars.network;
                netObj.hostname = name;    
                netObj.router = schemaVars.network.router ? schemaVars.network.router : 'other';
                networks.push(netObj);
            }

            var clear = "";
            // Generate configuration for parameters
            parameters.forEach(function(p) {
                var name = p.name;
                var path = getPath(p.path);
                var args = p.args;
                var argString = ``; // append config command arguments
                var argVals;
                // if it has a configuration for the parameter set it
                var obj = schemaVars;
                try {
                    for (p in path) {
                        obj = obj[path[p]];
                    }
                }
                catch (e) {
                    // console.log(e);
                    // return; 
                }
                if (Array.isArray(obj) && obj.length > 0) {
                    for (var i = 0; i < obj.length; i++) {
                        argVals = [];
                        for (var j = 0; j < args.length; j++) {
                            try {
                                var argval = obj[i][args[j]].toString(); // to catch false/boolean values
                                // explicit lookup for vlans with IDs; use ID instead of alias
                                if (name == 'net' && args[j] == 'vlan') {
                                    if (edgeArray.filter(function(e) { return e.name == argval; }).length > 0) {
                                        argval = edgeArray.filter(function(e) { return e.name == argval; })[0].id;
                                    }
                                }
                                if (argval && argval != '') {
                                    argVals.push(argval); 
                                }
                            }
                            catch (e) {
                                // console.log(e);
                            }
                        }
                        argString += argVals.join(',');
                        if (i < obj.length - 1 && obj.length != 1) {
                            argString += ` `;
                        }
                    }
                }
                else if (typeof obj !== 'undefined') {
                    argVals = [];
                    for (var i = 0; i < args.length; i++) {
                        try {
                            var argval = obj[args[i]].toString(); // to catch false/boolean values
                            if (argval && argval != '') {
                                argVals.push(argval);
                            }
                        }
                        catch (e) {
                            // console.log(e);
                        }
                    }
                    argString += argVals.join(',');
                }
                var value = argString;
                if (name == 'hostname' && schemaVars.type == 'kvm') value = '';
                // If new config for a parameter exists, set it
                if (prev_dev[name] != value && value != ''){ //  && name != "net"
                    prev_dev[name] = value;
                    config += `vm config ${name} ${value} \n`;
                }
                else if (prev_dev.hasOwnProperty(name) && value == '') { // && name != "net" 
                    // If there is no configuration for a parameter and the previous device had one clear it
                    delete prev_dev[name];
                    clear += `clear vm config ${name}\n`;
                }
            });

            config += clear;
            if (schemaVars.type == "container") {config+=`vm launch container ${name}\n\n`;}
            else {config+=`vm launch ${schemaVars.type} ${name}\n\n`;}
            if (typeof schemaVars.miniccc_commands !== 'undefined') {
                for (var i = 0; i < schemaVars.miniccc_commands.length; i++) {
                    if (schemaVars.miniccc_commands[i] != '') {
                        miniccc_commands.push(schemaVars.miniccc_commands[i]);
                    }
                }
            }
        });
        textarea.value = config;
        textarea.value += "## Starting all VM's\nvm start all\n";

        // router commands
        // console.log(networks);
        if (networks.length > 0) {
            textarea.value += "\n## router commands\n";
        }
        networks.forEach(network => {
            try {
                var routerType = network.router;
                for (var i = 0; i < network.interfaces.length; i++) {
                    var hostname = network.hostname;
                    var iface = null;
                    var ipAddr = null;
                    var ipMask = null;
                    var gateway = null;
                    var dhcp = false;
                    if (network.interfaces[i].proto) {
                        if (network.interfaces[i].proto == 'dhcp') {
                            dhcp = true;
                        }
                    }
                    if (network.interfaces[i].name) {
                        iface = network.interfaces[i].name;
                    }
                    if (network.interfaces[i].address) {
                        ipAddr = network.interfaces[i].address;
                    }
                    if (network.interfaces[i].mask) {
                        ipMask = network.interfaces[i].mask;
                    }
                    if (network.interfaces[i].gateway) {
                        gateway = network.interfaces[i].gateway;
                    }
                    if (iface && ((ipAddr && ipMask) || dhcp)) {
                        if (routerType == 'minirouter') {
                            var ipString =  !dhcp ? (ipAddr + '/' + ipMask) : 'dhcp';
                            textarea.value += `router ${hostname} ${iface} network ${ipString}\n`;
                        }
                        else {
                            if (!dhcp) {
                                textarea.value += `cc exec ip addr add ${ipAddr}/${ipMask} dev ${iface}\n`;
                            }
                            else {
                                textarea.value += `cc exec dhclient ${iface}\n`;
                            }
                        }
                    }
                    if (gateway) {
                        if (routerType == 'minirouter') {
                            textarea.value += `router ${hostname} gw ${gateway}\n`;
                        }
                        else {
                            textarea.value += `cc exec ip route add 0.0.0.0/0 via ${gateway}\n`;
                        }
                    }
                }
                if (routerType == 'minirouter') {
                    textarea.value += `router ${hostname} commit\n`;
                }
            }
            catch (e) {
                // console.log(e);
            }
        });

        // miniccc commands
        if (miniccc_commands.length > 0) textarea.value += "\n## miniccc commands\n"
        for(var i = 0; i < miniccc_commands.length; i++) {
            textarea.value += miniccc_commands[i]+"\n";
            // if (i == miniccc_commands.length - 1) {textarea.value += "\n";}
        }

        // apply experiment variables
        // console.log(window.experiment_vars);
        if (window.experiment_vars != undefined)
        {
            for (var i = 0; i < window.experiment_vars.length; i++)
            {
                var name = window.experiment_vars[i].name;
                var value = window.experiment_vars[i].value;

                var name = new RegExp('\\$'+name, 'g');
                textarea.value = textarea.value.replace(name, value);
            }
        }
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
                progressInit(document.getElementsByClassName('geDiagramContainer')[0]);

                // a little hacky, but need to delay to allow spinner to render, since
                // command execution wrecks DOM with large topos (TODO: use webworkers/async/etc.?)
                setTimeout(function() {
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

                    // console.log(cmds);
                    
                    var responseDlg = new MiniResponseDialog(editorUi);
                    $.post('/commands', JSON.stringify(cmds), function(resp){
                      editorUi.showDialog(responseDlg.container, 600, 600, true, false, null, false);
                      responseDlg.init();
                      for (let i = 0; i < resp.length; i++) {
                        let rs  = resp[i];
                        let cmd  = cmds[i];
                        responseDlg.appendRow(cmd.command, rs);
                      }
                    }, "json");

                    progressDestroy();
                    editorUi.hideDialog();
                }, 300);
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
    div.style.overflow = 'auto';

    var header = document.createElement('h2');
    header.textContent = "Minimega Response";
    header.style.marginTop = "0";
    header.style.marginBottom = "10px";

    var tdiv = document.createElement('div');
    tdiv.style['padding-right'] = "8px";
    // tdiv.style.overflowY = 'scroll';
    // tdiv.style.height = '500px';

    var table = document.createElement('table');
    table.setAttribute('wrap', 'off');
    table.setAttribute('spellcheck', 'false');
    table.setAttribute('autocorrect', 'off');
    table.setAttribute('autocomplete', 'off');
    table.setAttribute('autocapitalize', 'off');
    // table.style.overflow = 'auto';
    table.style.resize = 'none';
    table.style.width = '100%';
    // table.style.height = '100%';
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

    var okBtn = mxUtils.button(mxResources.get('ok'), function()
    {
        editorUi.hideDialog();
    });

    okBtn.className = 'geBtn gePrimaryBtn';

    var buttons = document.createElement('div');
    buttons.style.cssText = 'position:absolute;left:30px;right:30px;text-align:right;bottom:15px;height:40px;border-top:1px solid #ccc;padding-top:20px;margin-bottom:15px;'
    
    if (editorUi.editor.cancelFirst)
    {
        buttons.appendChild(cancelBtn);
        buttons.appendChild(okBtn);
    }
    else
    {
        buttons.appendChild(okBtn);
        buttons.appendChild(cancelBtn);
    }
    
    formdiv.appendChild(buttons);

    this.container = div;
};

/**
 *
 */
MiniResponseDialog.showNewWindowOption = true;


/**
 * Constructs a new JSONEditor for experiment variables
 */

// set default experiment variable values
window.experiment_vars = [
    {name: "IMAGEDIR", value:"/images"},
    {name: "DEFAULT_MEMORY", value:"2048"},
    {name: "DEFAULT_VCPU", value:"1"},
];

var VariablesDialog = function(ui)
{

    const graph = ui.editor.graph;
    const schema = ui.schemas['vars']; // set schema
    // console.log('this is schema'), console.log(schema);

    var startval = window.experiment_vars;

    // Set JSONEditor and config options based on schema and cell type
    var loadConfig = function () {

        this.config = {
            schema: schema,
            startval: startval,
            ajax: true,
            mode: 'tree',
            modes: ['code', 'text', 'tree'],
            show_errors: 'always',
            theme: 'bootstrap3',
            iconlib: 'spectre',
            disable_edit_json: true,
            disable_properties: true
        };

        createEditor();
    };

    var createEditor = function() {

        var div = document.createElement('div');
        div.style['overflow-y'] = 'auto';

        var editorContainer = document.createElement('div');
        editorContainer.setAttribute('id', 'jsoneditor');
        editorContainer.style.height = '100%';
        editorContainer.style.position = 'relative';
        editorContainer.style['max-width'] ="600px";

        const jsoneditor = new JSONEditor(editorContainer, this.config);

        div.appendChild(editorContainer);

        var cancelBtn = mxUtils.button(mxResources.get('cancel'), function()
        {
            ui.hideDialog.apply(ui, arguments);
            jsoneditor.destroy();
        });
        
        cancelBtn.className = 'geBtn';
        
        var applyBtn = mxUtils.button(mxResources.get('apply'), function()
        {
            const errors = jsoneditor.validate();
            var validForm = (errors.length) ? false : true;

            if (validForm) {
                try
                {
                    ui.hideDialog.apply(ui, arguments);
                    var updatedNode = jsoneditor.getEditor('root').value; // get current node's JSON (from JSONEditor)
                    window.experiment_vars = updatedNode;
                }
                catch (e)
                {
                    mxUtils.alert(e);
                }

                jsoneditor.destroy();
            }
            else {
                mxUtils.alert(JSON.stringify(errors));
            }

        });

        applyBtn.className = 'geBtn gePrimaryBtn';

        jsoneditor.on('change',() => {
            const errors = jsoneditor.validate();
            if (errors.length) {applyBtn.setAttribute('disabled', 'disabled');}
            else {applyBtn.removeAttribute('disabled');}
        });
        
        var buttons = document.createElement('div');
        buttons.style.cssText = 'position:absolute;left:30px;right:30px;text-align:right;bottom:15px;height:40px;border-top:1px solid #ccc;padding-top:20px;margin-bottom:15px;'
        
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

        // show dialog only after editor is created
        ui.showDialog(this.container, 480, 420, true, false, null, false); 

    };

    this.init = function()
    {
        loadConfig();
    };

};

// import JSON and parse to graph model
var ImportJSONDialog = function(graph, ui) {

    var inputElement = document.createElement('input');
    inputElement.type = "file";
    inputElement.id = "jsonimport";
    inputElement.name = "jsonimport";
    inputElement.style['margin-bottom'] = "15px";
    // event listerner for file selection
    inputElement.addEventListener('change', onChange);

    var div = document.createElement('div');

    var header = document.createElement('h2');
    header.textContent = "Import JSON";
    header.style.marginTop = "0";
    header.style.marginBottom = "10px";
    header.style.textAlign = 'left';

    var editorContainer = document.createElement('div');
    editorContainer.setAttribute('id', 'jsoneditor');
    editorContainer.style.height = '100%';
    editorContainer.style.position = 'relative';
    // editorContainer.style['max-width'] ="600px";
    // editorContainer.style.display = 'none';

    var textarea = document.createElement('textarea');
    textarea.setAttribute('readonly', 'readonly');
    // textarea.value = JSON.stringify(json, null, 2);
    textarea.setAttribute('id', 'jsonString');
    textarea.setAttribute('wrap', 'off');
    textarea.setAttribute('spellcheck', 'false');
    textarea.setAttribute('autocorrect', 'off');
    textarea.setAttribute('autocomplete', 'off');
    textarea.setAttribute('autocapitalize', 'off');
    textarea.style.overflow = 'auto';
    textarea.style.resize = 'none';
    textarea.style.width = '100%';
    textarea.style.height = '420px';
    textarea.style.lineHeight = 'initial';
    textarea.style.marginBottom = '16px';

    editorContainer.append(textarea);

    div.appendChild(header);
    div.appendChild(inputElement);
    div.appendChild(editorContainer);

    var cancelBtn = mxUtils.button(mxResources.get('cancel'), function()
    {
        ui.hideDialog.apply(ui, arguments);
        // jsoneditor.destroy();
    });
    
    cancelBtn.className = 'geBtn';
    cancelBtn.innerHTML = 'Close';
    
    var applyBtn = mxUtils.button(mxResources.get('apply'), function()
    {
        try
        {
            // generate diagram here
            ui.hideDialog.apply(ui, arguments);
            progressInit(document.getElementsByClassName('geDiagramContainer')[0]);
            // a little hacky, but need to delay to allow spinner to render, since
            // diagram generation wrecks DOM with large topos (TODO: use webworkers/async/etc.?)
            setTimeout(function() {
                generateDiagram(graph, JSON.parse(textarea.value));
            }, 300);
            
            
        }
        catch (e)
        {
            mxUtils.alert(e);
        }

        // jsoneditor.destroy();

    });

    applyBtn.className = 'geBtn gePrimaryBtn';

    var buttons = document.createElement('div');
    buttons.style.cssText = 'position:absolute;left:30px;right:30px;text-align:right;bottom:15px;height:40px;padding-top:20px;margin-bottom:15px;'

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

    // read JSON
    function onChange(event) {
        var reader = new FileReader();
        reader.onload = onReaderLoad;
        reader.readAsText(event.target.files[0]);
    }
    // render beautified JSON on read
    function onReaderLoad(event){
        try {
            var obj = JSON.parse(event.target.result);
            textarea.value = JSON.stringify(obj, null, 2);
            applyBtn.removeAttribute('disabled');
        }
        catch (e) {
            applyBtn.setAttribute('disabled', 'disabled');
            textarea.value = "invalid";
            mxUtils.alert(e);
        }
    }

    // custom codec for nested objects
    class JsonCodec extends mxObjectCodec {
        constructor() {
          super((value)=>{});
        }
        encode(value) {
            const xmlDoc = mxUtils.createXmlDocument();
            const newObject = xmlDoc.createElement("Object");
            for(let prop in value) {
              newObject.setAttribute(prop, value[prop]);
            }
            return newObject;
        }
        decode(model) {
          return Object.keys(model.cells).map(
            (iCell)=>{
              const currentCell = model.getCell(iCell);
              return (currentCell.value !== undefined)? currentCell : null;
            }
          ).filter((item)=> (item !== null));
        }
    }

    // parser for JSON to graph model
    // generates XML diagram
    function generateDiagram(graph, json){
        console.log('generate diagram');

        graph.setConnectable(true);

        //TODO: write parser to generate diagram from imported JSON
        var enc = new JsonCodec();
        var jsonNodes = json.nodes || []; // get nodes from JSON
        var jsonVlans = json.vlans || []; // get edges/vlans from JSON
        var host_count = 0;

        let vertices = []; // stores nodes/vlan targets 
        let switches = []; // stores switches
        let edges = []; // stores edges/vlans
        let vlans = {}; // stores vlans as keys with id, name, and targets properties

        // must enforce some styling standarization for successful JSON mport
        const devices = ['switch', 'router', 'firewall', 'desktop', 'server', 'mobile'];
        const types = ['kvm', 'container'];
        const stencilsDir = window.STENCIL_PATH; // image dir
        const vertexStyleString = "shape=image;html=1;labelBackgroundColor=#ffffff;verticalAlign=top;verticalLabelPosition=bottom;image=";
        const edgeStyleString = "edgeStyle=orthogonalEdgeStyle;rounded=0;orthogonalLoop=1;jettySize=auto;html=1;verticalAlign=top;verticalLabelPosition=bottom;";

        // create nodes, switches, and edges with schemaVars user object attribute
        // for schema validation
        // TODO: implement schema validation
        graph.getModel().clear(); // clear current graph model
        const parent = graph.getDefaultParent();
        // TODO: assign layers programatically 
        // var hiddenLayer = graph.model.root.insert(new mxCell());
        // var visibleLayer = graph.model.root.insert(new mxCell());
        graph.getModel().beginUpdate();
        try {
            jsonNodes.map(
                (node, idx)=> {
                    // create node
                    var obj = {};
                    // if device prop exists and valid, use it; else, default is desktop
                    var device = node.device || 'desktop';
                    device = device.toLowerCase();
                    if (!devices.includes(device)) {
                        device = 'desktop';
                    }
                    node.device = device;
                    // default to type kvm
                    var type = node.type || 'kvm';
                    type = type.toLowerCase();
                    if (!types.includes(type)) {
                        type = 'kvm';
                    }
                    node.type = type;
                    // build node styling string based on device and type
                    var imageDir = (type == 'kvm') ? "/virtual_machines" : "/containers";
                    var imageType = (type == 'kvm') ? "vm" : "container";
                    var styleString = vertexStyleString + stencilsDir + imageDir + "/" + device + "_blue_" + imageType + ".png";
                    obj.style = styleString;
                    obj.geometry = {width: 80, height: 80};
                    obj.value = {};
                    obj.value.schemaVars = JSON.stringify(node);
                    obj.value.label = node.general.hostname || device + '_device_' + host_count;
                    host_count++;
                    const xmlNode = enc.encode(obj.value);
                    var layer;
                    var ifaces;
                    try {
                        ifaces = node.network.interfaces; 
                    }
                    catch {
                        ifaces = null;
                    }
                    // TODO: move MGMT network to hidden layer
                    // try {
                    //     var hidden = ifaces.find(x => x.vlan === 'MGMT');
                    //     if (hidden) {
                    //         layer = hiddenLayer;
                    //     }
                    // }
                    // catch {
                    //     layer = visibleLayer;
                    // }
                    vertices[idx] = graph.insertVertex(parent, null, xmlNode, null, null, obj.geometry.width, obj.geometry.height, obj.style);
                    // if node has an interface w/ vlan, save vlan as key and capture vertex as target
                    // of implied switch
                    if (ifaces) {
                        for (var i = 0; i < ifaces.length; i++) {
                            var name = ifaces[i].vlan;
                            if (!vlans[name]) {
                                var vlan = {"id": 'auto', "name": name, "type": type};
                                vlan.targets = [];
                                vlans[name] = vlan;
                            }
                            vlans[name].targets.push(vertices[idx]);
                        }
                    }
                });
            var idx = 0;
            // create switch as source for each vlan and target
            for (let [key, value] of Object.entries(vlans)) {
                var type = value.type;
                var imageDir = (type == 'kvm') ? "/virtual_machines" : "/containers";
                var imageType = (type == 'kvm') ? "vm" : "container";
                var styleString = vertexStyleString + stencilsDir + imageDir + "/switch_blue_" + imageType + ".png";
                var vlanSwitch = {};
                vlanSwitch.style = styleString;
                vlanSwitch.geometry = {width: 80, height: 80};
                vlanSwitch.value = {};
                var obj = {};
                obj.device = 'switch';
                obj.hostname = value.name;
                vlanSwitch.value.label = value.name;
                obj.name = value.name;
                obj.id = value.id;
                vlanSwitch.value.schemaVars = JSON.stringify(obj);
                const xmlSwitch = enc.encode(vlanSwitch.value);
                switches[idx] = graph.insertVertex(parent, null, xmlSwitch, null, null, vlanSwitch.geometry.width, vlanSwitch.geometry.height, vlanSwitch.style);
                var source = switches[idx];
                // create edges
                for (var i = 0; i < value.targets.length; i++) {
                    var target = value.targets[i];
                    var edge = {};
                    edge.style = edgeStyleString;
                    edge.value = {};
                    edge.edge = true;
                    var obj = {};
                    obj.id = value.id;
                    obj.name = value.name;
                    edge.value.label = value.name
                    edge.value.schemaVars = JSON.stringify(obj);
                    const xmlEdge = enc.encode(edge.value);
                    edges[i] = graph.insertEdge(parent, null, xmlEdge, source, target, edge.style);
                }
                idx++;
            }
            // if JSON has vlans, set id values
            jsonVlans.map(
                (vlan, idx)=> {
                    var name = vlan.name;
                    var id = vlan.id;
                    for (var i = 0; i < switches.length; i++) {
                        var value = graph.getModel().getValue(switches[i]);
                        var schemaVars = JSON.parse(value.getAttribute('schemaVars'));
                        if (schemaVars.name = name) {
                            schemaVars.id = id;
                            value = value.cloneNode(true);
                            value.setAttribute('schemaVars', JSON.stringify(schemaVars));
                            graph.getModel().setValue(switches[i], value);
                            lookforvlan(graph, switches[i]);
                        }
                    }
                });
            // console.log(vertices), console.log(vlans), console.log(edges);
        }
        catch (e){
            console.log(e);
        }
        finally {
            // TODO: hide hidden layer
            // graph.model.setVisible(hiddenLayer, false);
            var first = new mxFastOrganicLayout(graph); // organic layout algorithm for graph layout
            first.forceConstant = 150; // TODO: dynamically compute force constant
            var second = new mxParallelEdgeLayout(graph);
            var layout = new mxCompositeLayout(graph, [first, second], first);
            layout.execute(parent);
            graph.getModel().endUpdate(); // Updates the display
            progressDestroy();
        }
    }

    this.init = function()
    {
        console.log('init import dialog');
    };

};

// render progress bar loading indicator
function progressInit(container) {
	// TODO: implement progress bar loading indicator
	// to replace spinner icon
    // var progressDiv = document.createElement('div');
    // progressDiv.setAttribute('id', 'progressDiv');
    // progressDiv.style.position = "fixed";
    // progressDiv.style.left = "33.33%";
    // progressDiv.style.top = "320px";
    // progressDiv.style.width = "33.33%";
    // progressDiv.style['z-index'] = "99999999";
    // progressDiv.setAttribute('class', "progress");
    // var progressBarDiv = document.createElement('div');
    // progressBarDiv.setAttribute('class', "progress-bar progress-bar-striped active");
    // progressBarDiv.setAttribute('role', "progressbar");
    // progressBarDiv.style.width = "100%";
    // progressBarDiv.innerHTML = "loading...";
    var faSpinnerDiv = document.createElement('i');
    faSpinnerDiv.setAttribute('id', 'loadingDiv')
    faSpinnerDiv.setAttribute('class', 'fa fa-circle-o-notch fa-spin fa-3x fa-fw');
    faSpinnerDiv.style.cssText = 'font-size:64px;color: #007bff;position: fixed;left: calc(50% - 32px);top: 50%;width: 64px;height: 64px;z-index: 9999999;';
    // progressDiv.appendChild(progressBarDiv);
    // container.appendChild(progressDiv);
    container.appendChild(faSpinnerDiv);
}

// destroy progress bar
function progressDestroy() {
    document.getElementById('loadingDiv').remove();
}
