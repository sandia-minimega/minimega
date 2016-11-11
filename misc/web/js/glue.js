"use strict";

// Config
var VM_REFRESH_THESHOLD = 500;      // Above this threshold, disable auto-refresh
var VM_REFRESH_ENABLE = true;       // Auto-refresh enabled?
var VM_REFRESH_TIMEOUT = 5000;      // How often the currently-displayed vms are updated (in millis)
var HOST_REFRESH_TIMEOUT = 5000;    // How often the currently-displayed hosts are updated (in millis)
var IMAGE_REFRESH_TIMEOUT = 5000;   // How often the currently-displayed screenshots are updated (in millis)
var COLOR_CLASSES = {
    BUILDING: "yellow",
    RUNNING:  "green",
    PAUSED:   "yellow",
    QUIT:     "blue",
    ERROR:    "red"
}

// Data
var lastImages = {};    // Cache of screenshots

// DataTables
var vmDataTable;
var hostDataTable;
var ssDataTable;


////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////
// Update tables
////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////


// Initialize the VM DataTable and set up an automatic reload
function initVMDataTable() {
    var vmDataTable = $('#vms-dataTable').DataTable({
        "ajax": function( data, callback, settings) {
            updateJSON('/vms.json', function(vmsData) {
                updateJSON('/vlans.json', function(vlansData) {
                    // create mapping of vlans to aliases for easier lookup
                    var aliases = {};
                    vlansData.forEach(function(vlan) {
                        aliases[vlan[2]] = vlan[1];
                    });
                    //console.log(aliases);

                    // insert VLAN aliases into VMs network data
                    vmsData.forEach(function(vm) {
                        vm["network"].forEach(function(network) {
                            network["Alias"] = aliases[network["VLAN"]];
                        });
                    });

                    // disable auto-refresh is there is more than 500 VMs
                    if (Object.keys(vmsData).length > VM_REFRESH_THESHOLD) {
                        VM_REFRESH_ENABLE = false;
                    }

                    var dataTablesData = {"data": vmsData};
                    callback(dataTablesData);
                });
            });
        },
        // custom DOM with Boostrap integration
        // http://stackoverflow.com/a/32253335
        "dom": 
            "<'row'<'col-sm-5'i><'col-sm-7'p>>" + 
            //"<'row'<'col-sm-3'l><'col-sm-6 text-center'B><'col-sm-3'f>>" +
            "<'row'<'col-sm-6'l><'col-sm-6'f>>" +
            "<'row'<'col-sm-12 text-center'B>>" +
            "<'row'<'col-sm-12'tr>>",
        "buttons": [
            'columnsVisibility',
        ],
        "autoWidth": false,
        "paging": true,
        "lengthChange": true,
        "lengthMenu": [
            [10, 25, 50, 100, 250, 500, -1],
            [10, 25, 50, 100, 250, 500, "All"]
        ],
        "pageLength": 500,
        "columns": [
            { "title": "Namespace", "data": "namespace", "visible": false, render: handleEmptyString },
            { "title": "Host", "data": "host", "visible": false },
            { "title": "Name", "data": "name" },
            { "title": "State", "data": "state" },
            { "title": "Type", "data": "type" },
            //{ "title": "ID", "data": "id" },
            { "title": "VCPUs", "data": "vcpus" },
            { "title": "Memory", "data": "memory" },
            { "title": "Disk", "data": null, "visible": false, render: renderDisksColumn },
            { "title": "VLAN", "data": "network", render: function(data, type, full, meta) {
                // create array with VLAN ID and alias zipped together
                var vlansWithAliases = data.map(function(e, i) {
                    return e["VLAN"] + " (" + e["Alias"] + ")";
                });
                return renderArray(vlansWithAliases, type, full, meta);
            } },
            { "title": "IPv4", "data": "network", render: renderArrayOfObjectsUsingKey("IP4") },
            { "title": "IPv6", "data": "network", "visible": false, render: renderArrayOfObjectsUsingKey("IP6") },
            { "title": "Taps", "data": "network", "visible": false, render: renderArrayOfObjectsUsingKey("Tap") },
            { "title": "Tags", "data": "tags", "visible": false, render: renderFilteredObject(function(key) {
                return key != 'minirouter_log';
            }) },
            { "title": "Active CC", "data": "activecc", "visible": false },
            {
                "title": "VNC",
                "data": "name",
                render:  function ( data, type, full, meta ) {
                    return '<a href="connect/'+data+'" target="_blank">Connect</a>';
                }
            },
        ],
        "order": [[ 0, 'asc' ], [ 1, 'asc' ]],
        /*initComplete: function(){
            var api = this.api();
            api.buttons().container().appendTo( '#' + api.table().container().id + ' .col-sm-6:eq(0)' );  
        }*/
    });

    
    // Create second button group for other functionality
    /*
    new $.fn.dataTable.Buttons( vmDataTable, {
        buttons: [
            {
                extend: 'copyHtml5',
                text: 'Copy to clipboard'
            },
            {
                extend: 'csvHtml5',
                text: 'Download CSV'
            },
        ]
    } );
    vmDataTable.buttons( 1, null ).container()
        .appendTo('#vms-dataTable_wrapper .col-sm-6:eq(0)');
    */

    if (VM_REFRESH_ENABLE && VM_REFRESH_TIMEOUT > 0) {
        setInterval(function() {
            vmDataTable.ajax.reload(null, false);
        }, VM_REFRESH_TIMEOUT);
    }
}


// Initialize the Host DataTable and set up an automatic reload
function initHostDataTable() {
    var hostDataTable = $('#hosts-dataTable').DataTable({
        "ajax": {
            "url": "hosts.json",
            "dataSrc": ""
        },
        "dom": 
            "<'row'<'col-sm-5'i><'col-sm-7'p>>" + 
            //"<'row'<'col-sm-3'l><'col-sm-6 text-center'B><'col-sm-3'f>>" +
            "<'row'<'col-sm-6'l><'col-sm-6'f>>" +
            "<'row'<'col-sm-12 text-center'B>>" +
            "<'row'<'col-sm-12'tr>>",
        "buttons": [
            'columnsVisibility'
        ],
        "autoWidth": false,
        "paging": true,
        "lengthChange": true,
        "lengthMenu": [
            [25, 50, 100, 200, -1],
            [25, 50, 100, 200, "All"]
        ],
        "pageLength": -1,
        "columns": [
            { "title": "Name" },
            { "title": "CPUs" },
            { "title": "Load", render: function(data, type, full, meta) {
                var loads = data.split(" ");
                var cpus = parseInt(full[1]);
                var loadsOverCPUsHtml = loads.map(function(load) {
                    return colorSpanWithThresholds(load, load, 1.5*cpus, 1.0*cpus);
                });
                return loadsOverCPUsHtml.join(" ");
            } },
            { "title": "Memory Used", render: function(data, type, full, meta) {
                var memUsed = parseInt(full[3]);
                var memTotal = parseInt(full[4]);
                var memUnits = full[4].replace(/[0-9]/g, '');
                var text = memUsed + "/" + memTotal + memUnits;
                var memRatio = memUsed / memTotal;
                return colorSpanWithThresholds(text, memRatio, 0.9, 0.8);
            } },
            { "title": "Memory Total", visible: false },
            { "title": "Bandwidth" },
            { "title": "VMs" },
            { "title": "VMs (all)" },
            { "title": "Uptime" , render: function(data, type, full, meta) {
                // calculate days separately
                var seconds = parseInt(data);
                var days = Math.floor(seconds / 86400);
                seconds -= days * 86400;
                return days + " days " + new Date(seconds * 1000).toISOString().substr(11, 8);
            } },
        ],
        "order": [[ 0, 'asc' ]]
    });
    hostDataTable.draw();

    if (HOST_REFRESH_TIMEOUT > 0) {
        setInterval(function() {
            hostDataTable.ajax.reload(null, false);
        }, HOST_REFRESH_TIMEOUT);
    }
}


// Update the Screenshot table with new data
function updateScreenshotTable(vmsData) {

    var imageUrls = Object.keys(lastImages);
    for (var i = 0; i < imageUrls.length; i++) {
        if (lastImages[imageUrls[i]].used === false) {
            delete lastImages[imageUrls[i]];
        } else {
            lastImages[imageUrls[i]].used = false;
        }
    }

    // Create the HTML element for each screenshot block
    // img has default value of null (http://stackoverflow.com/questions/5775469/)
    var model = $('                                                          \
        <td>                                                                 \
            <a class="connect-vm-wrapper" target="_blank">                   \
            <div class="thumbnail">                                          \
            <img src="images/ss_unavailable.svg" style="width: 300px; height: 225px;"> \
            <div class="screenshot-state"></div>                             \
            <div class="screenshot-label-host grey"></div>                   \
            <div class="screenshot-label grey"></div>                        \
            <div class="screenshot-connect grey">Click to connect</div>      \
            </div>                                                           \
            </a>                                                             \
        </td>                                                                \
    ');

    // Fill out the above model for each individual VM info and push into a list
    var screenshotList = [];
    for (var i = 0; i < vmsData.length; i++) {
        var toAppend = model.clone();
        var vm = vmsData[i];

        toAppend.find("h3").text(vm.name);
        //toAppend.find("a.connect-vm-button").attr("href", connectURL(vm));
        toAppend.find("a.connect-vm-wrapper").attr("href", connectURL(vm));
        toAppend.find("img").attr("data-url", screenshotURL(vm, 300));
        toAppend.find(".screenshot-state").addClass(COLOR_CLASSES[vm.state]).html(vm.state);
        toAppend.find(".screenshot-label").html(vm.name);
        toAppend.find(".screenshot-label-host").html("Host: " + vm.host);
        //if (vm.type != "kvm") toAppend.find(".connect-vm-button").css("visibility", "hidden");

        screenshotList.push({
            "name": vm.name,
            "host": vm.host,
            "state": vm.state,
            "model": toAppend.get(0).outerHTML,
            "vm": vm,
        });
    }

    // Push the list to DataTable
    if ($.fn.dataTable.isDataTable("#screenshots-list")) {
        var table = $("#screenshots-list").dataTable();
        table.fnClearTable(false);
        if (screenshotList.length > 0) {
            table.fnAddData(screenshotList, false);
        }
        table.fnDraw(false);
    } else {
        var table = $("#screenshots-list").DataTable({
            "autoWidth": false,
            "paging": true,
            "lengthChange": true,
            "lengthMenu": [
                [12, 24, 48, 96, -1],
                [12, 24, 48, 96, "All"]
            ],
            "pageLength": 12,
            "data": screenshotList,
            "columns": [
                { "title": "Name", "data": "name", "visible": false },
                { "title": "State", "data": "state", "visible": false },
                { "title": "Host", "data": "host", "visible": false },
                { "title": "Model", "data": "model", "searchable": false },
                { "title": "VM", "data": "vm", "visible": false, "searchable": false },
            ],
            "createdRow": loadOrRestoreImage
        });
    }
}


////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////
// Utility functions
////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////


// Get latest JSON from URL and pass it to a callback
function updateJSON (url, callback) {
    $.getJSON(url)
        .done(callback)
        .fail(function( jqxhr, textStatus, error) {
            var err = textStatus + ", " + error;
            console.warn( "Request Failed: " + err );
    });
}


function colorSpanWithThresholds(text, value, thresholdRed, thresholdYellow) {
    var spanClass = "";
    if (value > thresholdRed) {
        spanClass = "red";
    } else if (value > thresholdYellow) {
        spanClass = "yellow";
    }

    return "<span class='" + spanClass + "'>" + text + "</span>";
}


// Generate the appropriate URL for requesting a screenshot
function screenshotURL (vm, size) {
    return "screenshot/" + vm.host + "/" + vm.id + ".png?size=" + size;
}


// Generate the appropriate URL for a connection
function connectURL (vm) {
    return "connect/" + vm.name
}


// Add more cowbell
function initCowbell () {
    var audioElement = document.createElement('audio');
    audioElement.setAttribute('src', 'images/cow_and_bell_1243222141.mp3');
    $('#nav-container').dblclick(function() {
        audioElement.currentTime = 0;
        audioElement.play();
    });
    console.log("Added more cowbell.");
}


// Get the screenshot for the requested row,
// or restore it from the cache of screenshots if available
function loadOrRestoreImage (row, data, displayIndex) {
    // Skip if it is a container-type VM
    if (data.vm.type === "container") {
        return;
    }

    var img = $('img', row);
    var url = img.attr("data-url");

    if (Object.keys(lastImages).indexOf(url) > -1) {
        img.attr("src", lastImages[url].data);
        lastImages[url].used = true;
    }

    var requestUrl = url + "&base64=true" + "&" + new Date().getTime();

    $.get(requestUrl)
        .done(function(response) {
            lastImages[url] = {
                data: response,
                used: true
            };

            img.attr("src", response);
        })
        .fail(function( jqxhr, textStatus, error) {
            var err = textStatus + ", " + error;
            console.warn( "Request Failed: " + err );
    });
}

function renderDisksColumn(data, type, full, meta) {
    var html = [];
    var keys = [];
    if (data.type === "container") {
        var keys = ['container_fspath', 'container_preinit', 'container_init'];
    } else if (data.type === "kvm") {
        var keys = ['kvm_initrdpath', 'kvm_kernelpath', 'kvm_diskpaths'];
    }

    for (var i = 0; i < keys.length; i++) {
        html.push("<em>" + keys[i] + ":</em> " + handleEmptyString(data[keys[i]]));
    }

    return html.join("<br />");
}

function renderArray(data, type, full, meta) {
    var html = [];
    for (var i = 0; i < data.length; i++) {
        html.push(data[i]);
    }
    return handleEmptyString(html.join(", "));
}

function renderArrayOfObjectsUsingKey(key) {
    return function(data, type, full, meta) {
        return handleEmptyString(data.reduce(
            function (previous, current) {
                return previous.concat([handleEmptyString(current[key])]);
            }, []).join(", ")
        );
    };    
}

function renderFilteredObject(filterFn) {
    return function(data, type, full, meta) {
        var html = [];
        var keys = Object.keys(data).filter(filterFn);
        for (var i = 0; i < keys.length; i++) {
            html.push("<em>" + keys[i] + ":</em> " + data[keys[i]]);
        }
        return handleEmptyString(html.join(", "));
    }
}

function renderObject(data, type, full, meta) {
    var html = [];
    var keys = Object.keys(data);
    for (var i = 0; i < keys.length; i++) {
        html.push("<em>" + keys[i] + ":</em> " + data[keys[i]]);
    }
    return handleEmptyString(html.join(", "));
}

// Put an italic "null" in the table where there are fields that aren't set
function handleEmptyString (value, type) {
    if (
        (value === "") ||
        (value === null) ||
        (value === undefined) ||
        ((typeof(value) === "object") && (Object.keys(value).length === 0))
    ) {
        // don't print null if data is being used for a filter or sort operation
        // TODO not working as expected
        if (type === "filter" || type === "sort" || type === "type") {
            //console.log("bypassing handleEmptyString because: " + type);
            return "";
        } else {
            return '<span class="empty-string">null</span>';
        }
    }
    return value;
}