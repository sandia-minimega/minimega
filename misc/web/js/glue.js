"use strict";

// Config
var IMAGE_REFRESH_TIMEOUT = 10000;   // How often the currently-displayed screenshots are updated (in millis)
var HOST_REFRESH_TIMEOUT = 1000;    // How often the currently-displayed hosts are updated (in millis)
var VM_REFRESH_TIMEOUT = 1000;      // How often the currently-displayed vms are updated (in millis)
var NETWORK_COLUMN_INDEX = 5;       // Index of the column with network info (needs to have values strignified)
var IP4_COLUMN_INDEX = 6;           // Index of the column with IP4 info (needs to have values strignified)
var IP6_COLUMN_INDEX = 7;           // Index of the column with IP6 info (needs to have values strignified)
var TAP_COLUMN_INDEX = 8;           // Index of the column with tap info (needs to have values strignified)
var TAGS_COLUMN_INDEX = 10;         // Index of the column with tag info (needs to have values strignified)
var COLOR_CLASSES = {
    BUILDING: "yellow",
    RUNNING:  "green",
    PAUSED:   "yellow",
    QUIT:     "blue",
    ERROR:    "red"
}

// Data
var lastImages = {};    // Cache of screenshots

////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////
////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////
////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////


// Change which view (VMs, Hosts, Config) is currently shown
function setView () {
    var view = "#vms";
    if (window.location.hash) view = window.location.hash;
    $("a.current-view").removeClass("current-view");
    $('a[href$="' + view + '"]').addClass("current-view");

    $("div.current-view").removeClass("current-view");
    $(view).addClass("current-view");
}

// Callback for updating the host's information
function updateVMs () {
    $.getJSON('/vms')
        .done(function(vmsData) {
            updateVMsTables(vmsData);
        })
        .fail(function( jqxhr, textStatus, error) {
            var err = textStatus + ", " + error;
            console.warn( "Request Failed: " + err );
    });
}

// Callback for updating the host's information
function updateHosts () {
    $.getJSON('/hosts')
        .done(function(hostsData) {
            updateHostsTable(hostsData);
        })
        .fail(function( jqxhr, textStatus, error) {
            var err = textStatus + ", " + error;
            console.warn( "Request Failed: " + err );
    });
}

////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////
////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////
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

// Update the VMs dataTables with the new data.
function updateVMsTables(vmsData) {

    var imageUrls = Object.keys(lastImages);
    for (var i = 0; i < imageUrls.length; i++) {
        if (lastImages[imageUrls[i]].used === false) {
            delete lastImages[imageUrls[i]];
        } else {
            lastImages[imageUrls[i]].used = false;
        }
    }

    // Update the list of VMs datatable
    if ($.fn.dataTable.isDataTable('#vms-dataTable')) {
        var table = $('#vms-dataTable').dataTable();
        table.fnClearTable(false);
        if (vmsData.length > 0) {
            table.fnAddData(vmsData, false);
        }
        table.fnDraw(false);
    } else {
        var table = $('#vms-dataTable').DataTable({
            "aaData": vmsData,
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
        table.order([
            [ 0, 'asc' ],
            [ 1, 'asc' ]
        ]);
        table.draw();
    }

    
    // Update the screenshots list

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

    if ($.fn.dataTable.isDataTable("#screenshots-list")) {
        var table = $("#screenshots-list").dataTable();
        table.fnClearTable(false);
        if (screenshotList.length > 0) table.fnAddData(screenshotList, false);
        table.fnDraw(false);
    } else {
        var table = $("#screenshots-list").dataTable({
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
                    closureTable.fnDraw(false);
                }
            })(table), IMAGE_REFRESH_TIMEOUT)
        }

    }
}

// Update the hosts dataTable with new data
function updateHostsTable (hostsData) {
    if ($.fn.dataTable.isDataTable('#hosts-dataTable')) {
        var table = $('#hosts-dataTable').dataTable();
        table.fnClearTable(false);
        if (hostsData.length > 0) table.fnAddData(hostsData, false);
        table.fnDraw(false);
    } else {
        var table = $('#hosts-dataTable').DataTable({
            "aaData": hostsData,
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
        table.draw();
    }
}

////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////
////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////
////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////



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


// Turn a field into a string properly formatted for the table
function tableString (field, toplevel) {
    if (typeof(field) === "object") {
        if (Array.isArray(field)) {
            if (typeof(field[0]) === "object") {
                var accumulator = "";
                for (var i = 0; i < field.length; i++) {
                    accumulator += "<table style=\"float:right\">" + tableString(field[i], false) + "</table><br>";      // Sorry about this one.
                }
                return accumulator;
            } else if (field.length == 0) {
                return handleEmptyString();
            } else {
                var underscoredField = field.map(function (d) { return handleEmptyString(d); });
                return underscoredField.join(", ");
            }
        } else if ((field === null) || (Object.keys(field).length == 0)) {
            return handleEmptyString();
        } else if ((typeof(field) === "object") && (toplevel !== false)) {
            return "<table style=\"float:right\">" + tableString(field, false) + "</table>";
        } else {
            var toReturn = "";
            for (var key in field) {
                toReturn += "<tr><td>" + key + "</td><td>" + ((typeof(field[key]) === "object") ? tableString(field[key]) : handleEmptyString(field[key])) + "</td></tr>";
            }
            return toReturn;
        }
    } else {
        return String(handleEmptyString(field));
    }
}

function makeVNClink(vm) {
    return "<a target=\"_blank\" href=\"" + vncURL(vm) + "\">" + vm.host + ":" + (5900 + vm.id) + "</a>"
}

function addVNClink(parent, vm) {
    var newHtml = "";
    var oldHtml = parent.html();
    var row = $("<tr></tr>");
    $("<td></td>").appendTo(row).text("VNC");
    $("<td></td>").appendTo(row).html(makeVNClink(vm));
    newHtml += row.get(0).outerHTML;
    parent.html(newHtml + oldHtml);
}

// Build the DOM for the table
function makeTable (parent, data) {
    var newHtml = "";
    for (var key in data) {
        if ($.inArray(key, ["color", "uuid"]) === -1) {
            var row = $("<tr></tr>");
            $("<td></td>").appendTo(row).text(key);
            $("<td></td>").appendTo(row).html(tableString(data[key]));
            newHtml += row.get(0).outerHTML;
        }
    }

    parent.html(newHtml);
}





// Set the current view according to the hash on hash change
$(window).on('hashchange', function () {
    setView();
});

// Set the current view according to the hash on page load
// Begin updating the hosts dataTable
$(document).ready(function () {

    // Navigation init
    $("nav a").on("click", function (e) {
        $("a.current-view").removeClass("current-view");
        $(this).addClass("current-view");
        setView();
    });
    setView();

    updateVMs();
    if (VM_REFRESH_TIMEOUT > 0) {
        setInterval(updateVMs, VM_REFRESH_TIMEOUT);
    }

    updateHosts();
    if (HOST_REFRESH_TIMEOUT > 0) {
        setInterval(updateHosts, HOST_REFRESH_TIMEOUT);
    }
});