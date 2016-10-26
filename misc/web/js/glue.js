"use strict";

// Config
var VM_REFRESH_TIMEOUT = 2000;      // How often the currently-displayed vms are updated (in millis)
var HOST_REFRESH_TIMEOUT = 2000;    // How often the currently-displayed hosts are updated (in millis)
var IMAGE_REFRESH_TIMEOUT = 0;   // How often the currently-displayed screenshots are updated (in millis)
var NETWORK_COLUMN_INDEX = 4;       // Index of the column with network info (needs to have values strignified)
var IP4_COLUMN_INDEX = 5;           // Index of the column with IP4 info (needs to have values strignified)
var IP6_COLUMN_INDEX = 6;           // Index of the column with IP6 info (needs to have values strignified)
var TAP_COLUMN_INDEX = 7;           // Index of the column with tap info (needs to have values strignified)
var TAGS_COLUMN_INDEX = 9;         // Index of the column with tag info (needs to have values strignified)
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
// Request latest info from server
////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////


// Get latest VM information and pass it to a callback
function updateVMs (callback) {
    $.getJSON('/vms.json')
        .done(callback)
        .fail(function( jqxhr, textStatus, error) {
            var err = textStatus + ", " + error;
            console.warn( "Request Failed: " + err );
    });
}

// Get latest Host information and pass it to a callback
function updateHosts (callback) {
    $.getJSON('/hosts.json')
        .done(callback)
        .fail(function( jqxhr, textStatus, error) {
            var err = textStatus + ", " + error;
            console.warn( "Request Failed: " + err );
    });
}


////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////
// Update tables
////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////


// Initialize the VM DataTable and set up an automatic reload
function initVMDataTable() {
    var vmDataTable = $('#vms-dataTable').DataTable({
        "ajax": {
            "url": "vms.json",
            "dataSrc": ""
        },
        dom: 'Bfrtip',
        buttons: [
            'colvis'
        ],
        "autoWidth": false,
        "paging": true,
        aLengthMenu: [
            [25, 50, 100, 200, -1],
            [25, 50, 100, 200, "All"]
        ],
        iDisplayLength: -1,
        "aoColumns": [
            { "sTitle": "Host", "mDataProp": "host" },
            //{ "sTitle": "ID", "mDataProp": "id" },
            { "sTitle": "Memory", "mDataProp": "memory" },
            { "sTitle": "Name", "mDataProp": "name" },
            { "sTitle": "Network", "mDataProp": "network" },
            { "sTitle": "IPv4", "mDataProp": "network" },
            { "sTitle": "IPv6", "mDataProp": "network" },
            { "sTitle": "Taps", "mDataProp": "network" },
            { "sTitle": "State", "mDataProp": "state" },
            { "sTitle": "Tags", "mDataProp": "tags" },
            { "sTitle": "Type", "mDataProp": "type" },
            { "sTitle": "VCPUs", "mDataProp": "vcpus" }
        ],
        "fnRowCallback": flattenObjectValues
    });
    vmDataTable.order([
        [ 0, 'asc' ],
        [ 1, 'asc' ]
    ]);
    vmDataTable.draw();

    if (VM_REFRESH_TIMEOUT > 0) {
        setInterval(vmDataTable.ajax.reload, VM_REFRESH_TIMEOUT);
    }
}


// Initialize the Host DataTable and set up an automatic reload
function initHostDataTable() {
    var hostDataTable = $('#hosts-dataTable').DataTable({
        "ajax": {
            "url": "hosts.json",
            "dataSrc": ""
        },
        dom: 'Bfrtip',
        buttons: [
            'colvis'
        ],
        "autoWidth": false,
        "paging": true,
        aLengthMenu: [
            [25, 50, 100, 200, -1],
            [25, 50, 100, 200, "All"]
        ],
        iDisplayLength: -1,
        "aoColumns": [
            { "sTitle": "Name" },
            { "sTitle": "CPUs" },
            { "sTitle": "Load" },
            { "sTitle": "Memused" },
            { "sTitle": "Memtotal" },
            { "sTitle": "Bandwidth" },
            { "sTitle": "vms" },
            { "sTitle": "vmsall" },
            { "sTitle": "uptime" }
        ]
    });
    hostDataTable.draw();

    if (HOST_REFRESH_TIMEOUT > 0) {
        setInterval(hostDataTable.ajax.reload, HOST_REFRESH_TIMEOUT);
    }
}


// Update the Screenshot table with new data
function updateScreenshotTable(vmsData) {

    // Add "Connect" URLs into the data
    for (var i = 0; i < vmsData.length; i++) {
        var vm = vmsData[i];
        vm.vncURL = vncURL(vm);
        vm.screenshotURL = screenshotURL(vm, 300);
        //console.log(vm);
    }


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
        <td><div class="thumbnail">                                          \
            <img src="//:0" style="width: 300px; height: 225px;">            \
            <div class="caption">                                            \
                <h3></h3>                                                    \
                <p>                                                          \
                    <a class="btn btn-primary connect-vm-button" target="_blank">Connect</a> \
                    ' + /*<a href="#TODO" class="btn manage-vm-button">Manage</a>*/  '\
                </p>                                                         \
            </div>                                                           \
            <div class="screenshot-state"></div>                             \
        </div></td>                                                          \
    ');

    // Fill out the above model for each individual VM info and push into a list
    var screenshotList = [];
    for (var i = 0; i < vmsData.length; i++) {
        var toAppend = model.clone();
        var vm = vmsData[i];

        toAppend.find("h3").text(vm.name);
        toAppend.find("a.connect-vm-button").attr("href", vncURL(vm));
        toAppend.find("img").attr("data-url", screenshotURL(vm, 300));
        toAppend.find(".screenshot-state").addClass(COLOR_CLASSES[vm.state]).html(vm.state);

        //if (vm.type != "kvm") toAppend.find(".connect-vm-button").css("visibility", "hidden");

        screenshotList.push({
            "name": vm.name,
            "model": toAppend.get(0).outerHTML
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
            aLengthMenu: [
                [25, 50, 100, 200, -1],
                [25, 50, 100, 200, "All"]
            ],
            iDisplayLength: 200,
            "aaData": screenshotList,
            "aoColumns": [
                { "sTitle": "Name", "mDataProp": "name", "visible": false },
                { "sTitle": "Model", "mDataProp": "model", "searchable": false },
            ],
            "lengthMenu": [[6, 12, 30, -1], [6, 12, 30, "All"]],
            "fnRowCallback": loadOrRestoreImage
        });

        if (IMAGE_REFRESH_TIMEOUT > 0) {
            setInterval((function (closureTable) {
                return function () {
                    // Causes all the screenshot images to be re-requested and rendered
                    closureTable.fnDraw(false);
                }
            })(table), IMAGE_REFRESH_TIMEOUT)
        }

    }
}


////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////
// Utility functions
////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////


// Generate the appropriate URL for requesting a screenshot
function screenshotURL (vm, size) {
    return "./screenshot/" + vm.host + "/" + vm.id + ".png?size=" + size;
}


// Generate the appropriate URL for requesting a VNC connection
function vncURL (vm) {
    if (vm.type == "container") {
        return "./terminal#" + vm.name
    }
    return "./vnc#" + vm.name
}


// Get the screenshot for the requested row, or restore it from the cache of screenshots if available
function loadOrRestoreImage (row, data, displayIndex) {
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


// Stringify columns with object info
function flattenObjectValues (row, data, displayIndex) {
    var networkColumn = $("td:nth-child(" + NETWORK_COLUMN_INDEX + ")", row);
    var tapColumn = $("td:nth-child(" + TAP_COLUMN_INDEX + ")", row);
    var ip4Column = $("td:nth-child(" + IP4_COLUMN_INDEX + ")", row);
    var ip6Column = $("td:nth-child(" + IP6_COLUMN_INDEX + ")", row);
    var tagsColumn = $("td:nth-child(" + TAGS_COLUMN_INDEX + ")", row);

    ip4Column.html(handleEmptyString(data.network.reduce(
        function (previous, current) { return previous.concat([current.IP4]); },
        []
    ).join(", ")));

    ip6Column.html(handleEmptyString(data.network.reduce(
        function (previous, current) { return previous.concat([current.IP6]); },
        []
    ).join(", ")));

    tapColumn.html(handleEmptyString(data.network.reduce(
        function (previous, current) { return previous.concat([current.Tap]); },
        []
    ).join(", ")));

    networkColumn.html(handleEmptyString(data.network.reduce(
        function (previous, current) { return previous.concat([current.VLAN]); },
        []
    ).join(", ")));

    var tagsHTML = [];
    var tagsKeys = Object.keys(data.tags);
    for (var i = 0; i < tagsKeys.length; i++) {
        tagsHTML.push("<em>" + tagsKeys[i] + ":</em> " + data.tags[tagsKeys[i]]);
    }

    tagsColumn.html(handleEmptyString(tagsHTML.join(", ")));
}


// Put an italic "null" in the table where there are fields that aren't set
function handleEmptyString (value) {
    if (
        (value === "") ||
        (value === null) ||
        (value === undefined) ||
        ((typeof(value) === "object") && (Object.keys(value).length === 0))
    ) return '<span class="empty-string">null</span>';
    return value;
}