"use strict";

// Config
var VM_REFRESH_THESHOLD = 500;      // Above this threshold, disable auto-refresh of VMs data
var VM_REFRESH_ENABLE = true;       // Auto-refresh of VMs data enabled?
var VM_REFRESH_TIMEOUT = 5000;      // How often the currently-displayed vms are updated (in millis)
var HOST_REFRESH_TIMEOUT = 5000;    // How often the currently-displayed hosts are updated (in millis)
var IMAGE_REFRESH_THRESHOLD = 100;  // Above this threshold, disable auto-refresh of screenshots
var IMAGE_REFRESH_ENABLE = true;    // Auto-refresh of screenshots enabled?
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


// Initialize the `vm info` DataTable and set up an automatic reload
function initVMInfoDataTable() {
    var path = window.location.pathname;
    path = path.substr(0, path.indexOf("/vms"));

    var vmDataTable = $('#vms-dataTable').DataTable({
        "ajax": function( data, callback, settings) {
            updateJSON(path+'/vms/info.json', function(vmsData) {
                // disable auto-refresh there are too many VMs
                VM_REFRESH_ENABLE = Object.keys(vmsData).length <= VM_REFRESH_THESHOLD;

                // put into a structure that DataTables expects
                var dataTablesData = {"data": vmsData};

                callback(dataTablesData);
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
            { "title": "Host", "data": "host" },
            { "title": "Name", "data": "name" },
            { "title": "State", "data": "state" , render:  function ( data, type, full, meta ) {
				var res = "<span>"+data+"</span>";
				if (data == "BUILDING" || data == "PAUSED") {
					res += '<i class="fa fa-play-circle" id="'+full["name"]+'-start"></i>';
				} else if (data == "RUNNING") {
					res += '<i class="fa fa-pause-circle" id="'+full["name"]+'-stop"></i>';
				}
				res += '<i class="fa fa-times-circle" id="'+full["name"]+'-kill"></i>';

				return res;
			} },
            { "title": "Uptime", "data": "uptime", "visible": false },
            { "title": "Type", "data": "type", "visible": false },
            //{ "title": "ID", "data": "id" },
            { "title": "VCPUs", "data": "vcpus" },
            { "title": "Memory", "data": "memory" },
            { "title": "Disks", "data": null, "visible": false, render: renderDisksColumn },
            { "title": "VLAN", "data": "vlan" },
            { "title": "IPv4", "data": "ip" },
            { "title": "IPv6", "data": "ip6", "visible": false },
            { "title": "Taps", "data": "tap", "visible": false },
            { "title": "Tags", "data": "tags", "visible": false, render: renderFilteredObject(function(key) {
                return key != 'minirouter_log';
            }) },
            { "title": "Active CC", "data": "cc_active", "visible": false },
            {
                "title": "VNC",
                "data": "name",
                render:  function ( data, type, full, meta ) {
                    return '<a href="'+connectURL(full)+'" target="_blank">Connect</a>';
                }
            },
        ],
        "order": [[ 0, 'asc' ], [ 1, 'asc' ]],
        "stateSave": true,
        "stateDuration": 0
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

	// set onclick handler for all <i> to update VM
	$(document).on("click", "#vms-dataTable i", function() {
		var id = $(this).attr("id");
		var name = id.substr(0, id.lastIndexOf("-"));
		var action = id.substr(id.lastIndexOf("-")+1);

		var p = path;
		if (!p.endsWith("/")) {
			p += "/";
		}
		p += "vm/"+name+"/"+action;

		$.ajax({
			type: "POST",
			url: p,
			success: function() {
                vmDataTable.ajax.reload(null, false);
			},
		})
	});

    if (VM_REFRESH_TIMEOUT >= 1000) {
        setInterval(function() {
            if (VM_REFRESH_ENABLE) {
                vmDataTable.ajax.reload(null, false);
            }
        }, VM_REFRESH_TIMEOUT);
    }
}

// Initialize the `vm top` DataTable and set up an automatic reload
function initVMTopDataTable() {
    var path = window.location.pathname;
    path = path.substr(0, path.indexOf("/vms"));

    var vmDataTable = $('#vms-dataTable').DataTable({
        "ajax": function(data, callback, settings) {
            updateJSON(path+'/vms/top.json', function(vmsData) {
                // disable auto-refresh there are too many VMs
                VM_REFRESH_ENABLE = Object.keys(vmsData).length <= VM_REFRESH_THESHOLD;

                // put into a structure that DataTables expects
                var dataTablesData = {"data": vmsData};

                callback(dataTablesData);
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
            { "title": "Host", "data": "host" },
            { "title": "Name", "data": "name" },
            { "title": "Virtual", "data": "virt" },
            { "title": "Resident", "data": "res", "visible": false },
            { "title": "Shared", "data": "shr", "visible": false },
            { "title": "CPU", "data": "cpu" },
            { "title": "VCPU", "data": "vcpu" },
            { "title": "Time", "data": "time" },
            { "title": "Processes", "data": "procs" },
            { "title": "Rx", "data": "rx" },
            { "title": "Tx", "data": "tx" },
        ],
        "order": [[ 0, 'asc' ], [ 1, 'asc' ]],
        "stateSave": true,
        "stateDuration": 0
    });


    if (VM_REFRESH_TIMEOUT >= 1000) {
        setInterval(function() {
            if (VM_REFRESH_ENABLE) {
                vmDataTable.ajax.reload(null, false);
            }
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
            { "title": "Name", "data": "host" },
            { "title": "CPUs", "data": "cpus" },
            { "title": "Load", "data": "load", render: function(data, type, full) {
                var loads = data.split(" ");
                var cpus = parseInt(full["cpus"]);
                var loadsOverCPUsHtml = loads.map(function(load) {
                    return colorSpanWithThresholds(load, load, 1.5*cpus, 1.0*cpus);
                });
                return loadsOverCPUsHtml.join(" ");
            } },
            { "title": "Memory Used", "data": "memused", render: function(data, type, full) {
                var memUsed = parseInt(data);
                var memTotal = parseInt(full["memtotal"]);
                var memUnits = full["memtotal"].replace(/[0-9]/g, '');
                var text = memUsed + "/" + memTotal + memUnits;
                var memRatio = memUsed / memTotal;
                return colorSpanWithThresholds(text, memRatio, 0.9, 0.8);
            } },
            { "title": "Memory Total", "data": "memtotal", visible: false },
            { "title": "Rx Bandwidth", "data": "rx" },
            { "title": "Tx Bandwidth", "data": "tx" },
            { "title": "VMs", "data": "vms" },
            { "title": "VM Limit", "data": "vmlimit" },
            { "title": "CPU Commit", "data": "cpucommit" },
            { "title": "Mem Commit", "data": "memcommit" },
            { "title": "Net Commit", "data": "netcommit" },
            { "title": "Uptime", "data": "uptime", render: function(data, type, full, meta) {
                // calculate days separately
                var seconds = parseInt(data);
                var days = Math.floor(seconds / 86400);
                seconds -= days * 86400;
                return days + " days " + new Date(seconds * 1000).toISOString().substr(11, 8);
            } },
        ],
        "order": [[ 0, 'asc' ]],
        "stateSave": true,
        "stateDuration": 0
    });
    hostDataTable.draw();

    if (HOST_REFRESH_TIMEOUT > 0) {
        setInterval(function() {
            hostDataTable.ajax.reload(null, false);
        }, HOST_REFRESH_TIMEOUT);
    }
}


// Initialize the Namespace DataTable and set up an automatic reload
function initNamespacesDataTable() {
    console.log("initNamespacesDataTable");

    var table = $('#namespaces-dataTable').DataTable({
        "ajax": {
            "url": "namespaces.json",
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
            { "title": "Name", "data": "namespace", render:  function ( data, type, full, meta ) {
                return '<a href="/'+data+'/vms">'+data+'</a>';
            } },
            { "title": "VLANs", "data": "vlans", render:  function ( data, type, full, meta ) {
                if (data == "") {
                    data = "Inherited";
                }

                return '<a href="/'+full["namespace"]+'/vlans">'+data+'</a>';
            } },
            { "title": "Active", "data": "active" },
        ],
        "order": [[ 0, 'asc' ]],
        "stateSave": true,
        "stateDuration": 0
    });

    table.draw();

    if (HOST_REFRESH_TIMEOUT > 0) {
        setInterval(function() {
            table.ajax.reload(null, false);
        }, HOST_REFRESH_TIMEOUT);
    }
}

// Initialize the VLANs DataTable and set up an automatic reload
function initVLANsDataTable() {
    console.log("initVLANsDataTable");

    var path = window.location.pathname;
    path = path.substr(0, path.indexOf("/vlans"));

    var table = $('#vlans-dataTable').DataTable({
        "ajax": {
            "url": path+"/vlans.json",
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
            { "title": "Alias", "data": "alias" },
            { "title": "VLAN", "data": "vlan" },
        ],
        "order": [[ 0, 'asc' ]],
        "stateSave": true,
        "stateDuration": 0
    });

    table.draw();

    if (HOST_REFRESH_TIMEOUT > 0) {
        setInterval(function() {
            table.ajax.reload(null, false);
        }, HOST_REFRESH_TIMEOUT);
    }
}

// Initialize the Files DataTable and set up an automatic reload
function initFilesDataTable() {
    console.log("initFilesDataTable");

    var path = window.location.pathname;
    path = path.substr(0, path.indexOf("/files/"));

    var subdir = window.location.pathname;
    subdir = subdir.substr(subdir.indexOf("/files/")+"/files/".length);

    var table = $('#files-dataTable').DataTable({
        "ajax": {
            "url": path+"/files.json?path="+subdir,
            "dataSrc": function(data) {
                // Add '..' to data
                if (subdir != "") {
                    data.unshift({
                        "host": "",
                        "dir": "<dir>",
                        "name": "..",
                        "size": "",
                    });
                }

                return data;
            },
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
            { "title": "Host", "data": "host" },
            { "title": "Name", "data": "name", render:  function ( data, type, full, meta ) {
                var p = window.location.pathname;
                if (!p.endsWith("/")) {
                    p += "/";
                }
                var base = data.split(/[\\/]/).pop();
                if (full["dir"] != "") {
                    base += "/";
                }
                return '<a href="'+p+base+'">'+base+'</a>';
            } },
            { "title": "Size", "data": "size", render:  function ( data, type, full, meta ) {
                // From https://stackoverflow.com/a/22023833
                var exp = Math.log(data) / Math.log(1024) | 0;
                var result = (data / Math.pow(1024, exp)).toFixed(2);

                return result + ' ' + (exp == 0 ? 'bytes': 'KMGTPEZY'[exp - 1] + 'B');

            } },
        ],
        "order": [[ 0, 'asc' ]],
        "stateSave": true,
        "stateDuration": 0
    });

    table.draw();

    if (HOST_REFRESH_TIMEOUT > 0) {
        setInterval(function() {
            table.ajax.reload(null, false);
        }, HOST_REFRESH_TIMEOUT);
    }
}


// Initialize the Screenshot DataTable and set up an automatic reload
function initScreenshotDataTable() {
    var path = window.location.pathname;
    path = path.substr(0, path.indexOf("/tilevnc"));

    updateJSON(path+"/vms/info.json", updateScreenshotTable);

    if (IMAGE_REFRESH_TIMEOUT > 0) {
        setInterval(function() {
            if (IMAGE_REFRESH_ENABLE) {
                updateJSON(path+"/vms/info.json", updateScreenshotTable);
            }
        }, IMAGE_REFRESH_TIMEOUT);
    }
}


// Update the Screenshot table with new data
function updateScreenshotTable(vmsData) {

    // disable auto-refresh there are too many VMs
    IMAGE_REFRESH_ENABLE = Object.keys(vmsData).length <= IMAGE_REFRESH_THRESHOLD;

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
            "rowCallback": loadOrRestoreImage,
            "stateSave": true,
            "stateDuration": 0
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
    return "vm/" + vm.name + "/screenshot.png?size=" + size;
}


// Generate the appropriate URL for a connection
function connectURL (vm) {
    return "vm/" + vm.name + "/connect";
}


// Add more cowbell
function initCowbell () {
    var audioElement = document.createElement('audio');
    audioElement.setAttribute('src', '/images/cow_and_bell_1243222141.mp3');
    audioElement.volume = 0.1;
    document.querySelector('#nav-container').addEventListener('click', function (e) {
        if (e.detail === 3) {
            audioElement.currentTime = 0;
            audioElement.play();
        }
    });
    console.log("Added reduced cowbell.");
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
        var keys = ['filesystem', 'preinit', 'init'];
    } else if (data.type === "kvm") {
        var keys = ['initrd', 'kernel', 'disks'];
    }

    for (var i = 0; i < keys.length; i++) {
        html.push("<em>" + keys[i] + ":</em> " + handleEmptyString(data[keys[i]]));
    }

    return html.join("<br />");
}

function renderFilteredObject(filterFn) {
    return function(data, type, full, meta) {
        var jsonified = JSON.parse(data);
        var html = [];
        var keys = Object.keys(jsonified).filter(filterFn);
        for (var i = 0; i < keys.length; i++) {
            html.push("<em>" + keys[i] + ":</em> " + jsonified[keys[i]]);
        }
        return handleEmptyString(html.join(", "));
    }
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
