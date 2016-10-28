"use strict";

// Config
var VM_REFRESH_TIMEOUT = 2000;      // How often the currently-displayed vms are updated (in millis)
var HOST_REFRESH_TIMEOUT = 2000;    // How often the currently-displayed hosts are updated (in millis)
var IMAGE_REFRESH_TIMEOUT = 2000;   // How often the currently-displayed screenshots are updated (in millis)
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
            [25, 50, 100, 200, -1],
            [25, 50, 100, 200, "All"]
        ],
        "pageLength": -1,
        "columns": [
            {
                "title": "VNC",
                "data": "name",
                render:  function ( data, type, full, meta ) {
                    return '<a href="connect/'+data+'" target="_blank">Connect</a>';
                }
            },

            { "title": "Name", "data": "name" },
            { "title": "State", "data": "state" },
            { "title": "Host", "data": "host" },
            //{ "title": "ID", "data": "id" },
            { "title": "Memory", "data": "memory" },
            { "title": "Network", "data": "network", render: renderArrayOfObjectsUsingKey("VLAN") },
            { "title": "IPv4", "data": "network", render: renderArrayOfObjectsUsingKey("IP4") },
            { "title": "IPv6", "data": "network", render: renderArrayOfObjectsUsingKey("IP6") },
            { "title": "Taps", "data": "network", render: renderArrayOfObjectsUsingKey("Tap") },
            { "title": "Tags", "data": "tags", render: renderArrayOfObjects },
            { "title": "Type", "data": "type" },
            { "title": "VCPUs", "data": "vcpus" }
        ],
        "order": [[ 2, 'asc' ], [ 1, 'asc' ]],
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
            { "title": "Load" },
            { "title": "Memused" },
            { "title": "Memtotal" },
            { "title": "Bandwidth" },
            { "title": "vms" },
            { "title": "vmsall" },
            { "title": "uptime" }
        ],
        "order": [[ 0, 'asc' ]]
    });
    hostDataTable.draw();

    if (HOST_REFRESH_TIMEOUT > 0) {
        setInterval(hostDataTable.ajax.reload, HOST_REFRESH_TIMEOUT);
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
        toAppend.find("a.connect-vm-button").attr("href", connectURL(vm));
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
            "lengthChange": true,
            "lengthMenu": [
                [12, 24, 48, 96, -1],
                [12, 24, 48, 96, "All"]
            ],
            "pageLength": -1,
            "data": screenshotList,
            "columns": [
                { "title": "Name", "data": "name", "visible": false },
                { "title": "Model", "data": "model", "searchable": false },
            ],
            "createdRow": loadOrRestoreImage
        });
    }
}


////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////
// Utility functions
////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////


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

function renderArrayOfObjectsUsingKey(key) {
    return function(data, type, full, meta) {
        return handleEmptyString(data.reduce(
            function (previous, current) {
                return previous.concat([handleEmptyString(current[key])]);
            }, []).join(", ")
        );
    };    
}

function renderArrayOfObjects(data, type, full, meta) {
    var html = [];
    var keys = Object.keys(data);
    for (var i = 0; i < keys.length; i++) {
        html.push("<em>" + keys[i] + ":</em> " + data[keys[i]]);
    }
    return handleEmptyString(html.join(", "));
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