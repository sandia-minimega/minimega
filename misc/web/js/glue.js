"use strict";

var IMAGE_REFRESH_TIMEOUT = 5000;
var NETWORK_COLUMN_INDEX = 5;
var IP4_COLUMN_INDEX = 6;
var IP6_COLUMN_INDEX = 7;
var TAP_COLUMN_INDEX = 8;
var TAGS_COLUMN_INDEX = 10;
var COLOR_CLASSES = {
    BUILDING: "yellow",
    RUNNING:  "green",
    PAUSED:   "yellow",
    QUIT:     "blue",
    ERROR:    "red"
}
var hostData = [];
var hostString = "";

function setView () {
    var view = "#vms";
    if (window.location.hash) view = window.location.hash;
    $("a.current-view").removeClass("current-view");
    $('a[href$="' + view + '"]').addClass("current-view");

    $("div.current-view").removeClass("current-view");
    $(view).addClass("current-view");
}

function updateHosts () {
    d3.text("./hosts", function (error, info) {
        if (info != hostData) {
            if (error) return console.warn(error);

            hostString = info;
            hostData = JSON.parse(info);

            updateHostsTable();
        }
    });
}

////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////
////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////
////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////

function screenshotURL (vm, size) {
    return "./screenshot/" + vm.host + "/" + vm.id + ".png?size=" + size;
}

function vncURL (vm) {
    return "./vnc#" + vm.host + ":" + (5900 + vm.id) + ":" + vm.name
}

var lastImages = {};
function loadOrRestoreImage (row, data, displayIndex) {
    var img = $('img', row);
    var url = img.attr("data-url");

    if (Object.keys(lastImages).indexOf(url) > -1) {
        img.attr("src", lastImages[url].data);
        lastImages[url].used = true;
    }

    convertImgToBase64URL(url + "&" + new Date().getTime(), (function () {
        return function (base64) {
            lastImages[url] = {
                data: base64,
                used: true
            };
            img.attr("src", base64);
        };
    })());
}

// http://stackoverflow.com/a/20285053
function convertImgToBase64URL(url, callback, outputFormat){
    var img = new Image();
    img.crossOrigin = 'Anonymous';

    img.onload = function(){
        var canvas = document.createElement('CANVAS'),
        ctx = canvas.getContext('2d'), dataURL;
        canvas.height = this.height;
        canvas.width = this.width;
        ctx.drawImage(this, 0, 0);
        dataURL = canvas.toDataURL(outputFormat);
        callback(dataURL);
        canvas = null; 
    };

    img.src = url;
}

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

function updateTables () {

    var imageUrls = Object.keys(lastImages);
    for (var i = 0; i < imageUrls.length; i++) {
        if (lastImages[imageUrls[i]].used === false) {
            delete lastImages[imageUrls[i]];
        } else {
            lastImages[imageUrls[i]].used = false;
        }
    }

////// Update the main datatable

    if ($.fn.dataTable.isDataTable('#vms-dataTable')) {
        var table = $('#vms-dataTable').dataTable();
        table.fnClearTable(false);
        if (grapher.jsonData.length > 0) table.fnAddData(grapher.jsonData, false);
        table.fnDraw(false);
    } else {
        var table = $('#vms-dataTable').DataTable({
            "aaData": grapher.jsonData,
            "aoColumns": [
                { "sTitle": "Host", "mDataProp": "host" },
                { "sTitle": "ID", "mDataProp": "id" },
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

////// Update the VMs list

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
    for (var i = 0; i < grapher.jsonData.length; i++) {
        var toAppend = model.clone();
        var vm = grapher.jsonData[i];

        toAppend.find("h3").text(vm.name);
        toAppend.find("a.connect-vm-button").attr("href", vncURL(vm));
        toAppend.find("img").attr("data-url", screenshotURL(vm, 300));
        toAppend.find(".screenshot-state").addClass(COLOR_CLASSES[vm.state]).html(vm.state);
        
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
        setInterval((function (closureTable) {
            return function () {
                closureTable.fnDraw(false);
            }
        })(table), IMAGE_REFRESH_TIMEOUT)
    }
}

function updateHostsTable () {
    if ($.fn.dataTable.isDataTable('#hosts-dataTable')) {
        var table = $('#hosts-dataTable').dataTable();
        table.fnClearTable(false);
        if (hostData.length > 0) table.fnAddData(hostData, false);
        table.fnDraw(false);
    } else {
        var table = $('#hosts-dataTable').DataTable({
            "aaData": hostData,
            "aoColumns": [
                { "sTitle": "Name" },
                { "sTitle": "CPUs" },
                { "sTitle": "Load" },
                { "sTitle": "Memused" },
                { "sTitle": "Memtotal" },
                { "sTitle": "Bandwidth" }
            ]
        });
        table.draw();
    }
}

////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////
////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////
////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////

$(document).ready(function () {
    $("nav a").on("click", function (e) {
        $("a.current-view").removeClass("current-view");
        $(this).addClass("current-view");
        setView();
    });

    setView();
    setInterval(updateHosts, 750);
});

$(window).on('hashchange', function () {
    setView();
});

