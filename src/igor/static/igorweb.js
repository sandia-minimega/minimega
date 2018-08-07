// var numNodeCols = 16;
// var numNodes = 288;
// var reservations = [
//     {"nodes": [8, 9, 10, 11, 12 ,13, 14, 15]},
//     {"name": "jacob", "owner": "Thornton", "start": "Apr 25 09:37", "end": "Apr 30 09:37", "nodes": [1, 2, 3]},
//     {"name": "bird", "owner": "Larry", "start": "Apr 25 09:37", "end": "Apr 30 09:37", "nodes": [4, 5, 6, 7]},
//     {"name": "jacob", "owner": "Thornton", "start": "Apr 25 09:37", "end": "Apr 30 09:37", "nodes": [16, 17]},
//     {"name": "bird", "owner": "Larry", "start": "Apr 25 09:37", "end": "Apr 30 09:37", "nodes": [18]},
//     {"name": "jacob", "owner": "Thornton", "start": "Apr 25 09:37", "end": "Apr 30 09:37", "nodes": [19, 20, 30, 50]},
//     {"name": "bird", "owner": "Larry", "start": "Apr 25 09:37", "end": "Apr 30 09:37", "nodes": [60, 75, 288]},
//     {"name": "jacob", "owner": "Thornton", "start": "Apr 25 09:37", "end": "Apr 30 09:37", "nodes": [88, 89, 90, 93]},
//     {"name": "bird", "owner": "Larry", "start": "Apr 25 09:37", "end": "Apr 30 09:37", "nodes": [21, 22, 23, 24, 25]},
// ];
// console.log(reservations[0].Name);
// var specResults = [
//     {"start": "Apr 25 09:37", "end": "Apr 30 09:37"},
//     {"start": "Apr 25 09:37", "end": "Apr 30 09:37"},
//     {"start": "Apr 25 09:37", "end": "Apr 30 09:37"},
//     {"start": "Apr 25 09:37", "end": "Apr 30 09:37"},
//     {"start": "Apr 25 09:37", "end": "Apr 30 09:37"},
//     {"start": "Apr 25 09:37", "end": "Apr 30 09:37"},
//     {"start": "Apr 25 09:37", "end": "Apr 30 09:37"},
//     {"start": "Apr 25 09:37", "end": "Apr 30 09:37"},
//     {"start": "Apr 25 09:37", "end": "Apr 30 09:37"},
//     {"start": "Apr 29 09:37", "end": "Apr 35 09:37"}
// ];
// TODO: work with focusing
// var colors = {
//     "Reserved": {
//         "Down": {
//             "Unselected": "#ffdd9b",
//             "Selected": "#eab448",
//         },
//         "Up": {
//             "Unselected": "#ccdfff",
//             "Selected": "#3f73cc",
//         }
//     },
//     "Available": {
//         "Down": {
//             "Unselected": "#e85555",
//             "Selected": "#ffb5b5",
//         },
//         "Up": {
//             "Unselected": "#ccdfff",
//             "Selected": "#eab448",
//         }
//     }
// }
//     ["#ccdfff", "#3f73cc"],
//     ["#e1c8f7", "#a975d6"],
//     ["#ffb5b5", "#e85555"],
//     ["#ffdd9b", "#eab448"],
//     ["#ffdd9b", "#eab448"],
// ]
var response = "";

// selection functions
function getResIndexFromObj(obj) {
    return parseInt((obj.attr("id")).slice(3));
}

function getObjFromResIndex(index) {
    return $("#res" + index);
}

function getNodeIndexFromObj(obj) {
    return parseInt(obj.attr("id"));
}

function getObjFromNodeIndex(index) {
    return $("#" + index);
}

var selectedNodes = [];
var selectedRes = -1;
function select(obj) {
    if (obj.hasClass("active")) return;
    if (obj.hasClass("node")) {
        selectedNodes.push(getNodeIndexFromObj(obj));
        selectedNodes.sort((a, b) => a - b);
    } else {
        var selectedResTemp = getResIndexFromObj(obj);
        deselectTable((selectedResTemp === 0) ? 0 : -2);
        selectedRes = selectedResTemp;
        toggleNodes(reservations[selectedRes].Nodes);
    }
    obj.addClass("active");
    showActionBar();
}

function deselect(obj) {
    if (!obj.hasClass("active")) return;
    obj.removeClass("active");
    if (obj.hasClass("node")) {
        selectedNodes.splice(selectedNodes.indexOf(getNodeIndexFromObj(obj)), 1);
    } else {
        selectedRes = -1;
    }
    checkHideActionBar();
}

function toggleNodes(nodes) {
    for (var i = 0; i < nodes.length; i++) {
        toggle(getObjFromNodeIndex(nodes[i]));
    }
}

function selectNodes(startNode, endNode, toOn) {
    for (var i = 0; i <= Math.abs(endNode - startNode); i++) {
        var node = Math.min(endNode, startNode) + i;
        if (toOn) {
            select(getObjFromNodeIndex(node));
        } else {
            deselect(getObjFromNodeIndex(node));
        }
    }
}

function deselectGrid() {
    $(".node").removeClass("active");
    selectedNodes = [];
    checkHideActionBar()
}

function deselectTable(sr = -1) {
    $(".res").removeClass("active");
    selectedRes = sr;
    checkHideActionBar()
}

function toggle(obj) {
    if (obj.hasClass("active")) {
        deselect(obj);
        return true;
    } else {
        select(obj);
        return false;
    }
}

var resActionShown = false;
function showActionBar() {
    if (selectedRes > 0) {
        showResActions();
    } else {
        hideResActions();
    }
    $("#actionbar").addClass("active");
}

function checkHideActionBar() {
    if (selectedRes === -1 || selectedRes === 0) {
        hideResActions();
        if (selectedNodes.length === 0) {
            hideActionBar();
        }
    } else {
        showResActions();
    }
}

function hideActionBar() {
    $("#actionbar").removeClass("active");
}

function showResActions() {
    $(".resaction").fadeIn(200, function() {
        $(".resaction").show();
    });
}

function hideResActions() {
    $(".resaction").fadeOut(200, function() {
        $(".resaction").hide();
    });
}

function updateNodeListField(id = "dashw") {
    var list = "";
    for (var i = 0; i < selectedNodes.length; i++) {
        var node = selectedNodes[i];
        if (i != 0) {
            list += ", "
        }
        for (; i + 1 < selectedNodes.length && selectedNodes[i+1] === selectedNodes[i] + 1; i++);
        if (selectedNodes[i] != node) {
            list += node + "-" + selectedNodes[i];
            continue;
        }
        list += node;
    }
    $("#" + id).val(list);
}

function disable(elems, bool) {
    for (var i = 0; i < elems.length; i++) {
        $("#dash" + elems.charAt(i)).prop("disabled", bool);
        $("#dash" + elems.charAt(i) + "code").css("color", (bool ? "royalblue" : ""));
        var title = $("#dash" + elems.charAt(i) + "parent").prop("title");
        if (bool) {
            title = "Click to enable this field\n" + title;
        } else {
            title = title.slice(27);
        }
        $("#dash" + elems.charAt(i) + "parent").prop("title", title);
    }
}

function debug(message) {
    $("body").append(message + "\n");
}

function execute(onResponse) {
    response = "";
    $(".command").html(command);
    var result;
    $.get(
        "/run/",
        {run: command},
        function(data) {
            setTimeout(function() {
                response = JSON.parse(data);
                onResponse();
            }, 700)
        }
    );
}

function parseResult(successbar = true) {
    if (new Date().getTime() % 5 != 0) {
        result = [true, response];
    } else {
        result = [false, response];
    }
    $(".result").html(response.Message);
    if (response.Success) {
        if (successbar) {
            $("#successbar").addClass("active");
            setTimeout(function() {
                if (!$("#successbar").is(":hover")) {
                    $("#successbar").removeClass("active");
                }
                $("#successbar").mouseleave(function() {
                    setTimeout(function() {
                        if (!$("#successbar").is(":hover")) {
                            $("#successbar").removeClass("active");
                        }
                    }, 1000)
                })
            }, 5000);
        }
        return true;
    }
    $(".error").html(response.Message);
    $(".errorparent").show();
    return false;
}

function showLoader(obj) {
    obj.children().eq(0).css("visibility", "hidden");
    obj.children().eq(1).css("visibility", "visible");
    $(".dash, .edash, .pdash").prop("disabled", true);
    $(".igorbtn").prop("disabled", true);
    $(".cancel").prop("disabled", true);
}

function hideLoaders() {
    $(".loader").css("visibility", "hidden");
    $(".mdlcmdtext").css("visibility", "visible");
    $(".dash, .edash, .pdash").prop("disabled", false);
    $(".igorbtn").prop("disabled", false);
    $(".cancel").prop("disabled", false);
}


// populate node grid
var newcol = '<div class="col" style="padding: 0">' +
'    <div class="list-group" ';
for (var i = 0; i < numNodeCols; i++) {
    $('#nodegrid').append(newcol + 'id="col' + i + '"></div></div>');
}
var grid = '<div draggable="true" tabIndex="-1" style="width:100%; padding: 12px; padding-left: 0px; padding-right: 0px; cursor: pointer;" ';
for (var i = 1; i <= numNodes; i++) {
    col = (i - 1) % numNodeCols;
    var classes = ' class="list-group-item list-group-item-action node ';
    if (reservations[0].Nodes.includes(i)) {
        classes += "down ";
    } else {
        classes += "up ";
    }
    var reserved = false;
    for (var j = 1; j < reservations.length; j++) {
        if (reservations[j].Nodes.includes(i)) {
            reserved = true;
            classes += "reserved ";
            break;
        }
    }
    if (!reserved) {
        classes += "available ";
    }
    classes += "unselected ";
    classes += '" ';
    $("#col" + col).append(grid + classes + ' id="' + i +'">' + i + '</div>');
}

// node grid selections
$(".node").click(function(event) {
    deselectTable();
    toggle($(this));
});

$(".node").hover(function() {
    var node = getNodeIndexFromObj($(this));
    for (var i = 0; i < reservations.length; i++) {
        if (reservations[i].Nodes.includes(node)) {
            getObjFromResIndex(i).addClass("hover");
        };
    }
});

$(".node").mouseleave(function() {
    var node = getNodeIndexFromObj($(this));
    for (var i = 0; i < reservations.length; i++) {
        if (reservations[i].Nodes.includes(node)) {
            getObjFromResIndex(i).removeClass("hover");
        };
    }
});

// $(".node").prop("draggable", true);
var selectOn = true;
var firstDragNode = -1;
// // $(".node").on("dragstart", function( event ) {
// //     // deselectTable();
// //     // firstDragNode = $(event.target);
// //     // if ($(event.target).hasClass("active")) {
// //     //     selectOn = false;
// //     // } else {
// //     //     selectOn = true;
// //     // }
// //     // toggle($(event.target));
// // });
// TODO: configure dragging for firefox
// TODO: make sure dragging can be undone in the same movement
// TODO: fix hover, I don't like it right now
// TODO: disable drag from outside
var nodes = document.getElementsByClassName("node");
var nodeLeft;
var maxDragNode;
var minDragNode;
var lastDrag;
for (var i = 0; i < nodes.length; i++) {
    nodes[i].addEventListener("dragstart", function(e) {
        // console.log("dragstart");
        deselectTable();
        firstDragNode = $(event.target);
        maxDragNode = getNodeIndexFromObj(firstDragNode);
        minDragNode = getNodeIndexFromObj(firstDragNode);
        lastDrag = getNodeIndexFromObj(firstDragNode);
        if ($(event.target).hasClass("active")) {
            selectOn = false;
        } else {
            selectOn = true;
        }
        toggle($(event.target));
        var crt = this.cloneNode(true);
        crt.style.display = "none";
        document.body.appendChild(crt);
        e.dataTransfer.setDragImage(crt, 0, 0);
    }, false);
}

$(".node").on("dragover", function( event ) {
    // console.log(firstDragNode);
    if (firstDragNode === -1) return;
    var fromIndex = getNodeIndexFromObj(firstDragNode);
    var toIndex = getNodeIndexFromObj($(event.target));
    if (toIndex > minDragNode && toIndex < maxDragNode) {
        if (toIndex > minDragNode && toIndex < firstDragNode) {
            console.log("deselecta");
            selectNodes(minDragNode, toIndex, !selectOn);
        }
        if (toIndex > firstDragNode && toIndex < maxDragNode) {
            console.log("deselectb");
            selectNodes(maxDragNode, toIndex, !selectOn);
        }
    } else {
        selectNodes(fromIndex, toIndex, selectOn);
    }
    if (toIndex < firstDragNode && lastDrag >= firstDragNode) {
        console.log("switcha");
    }
    if (toIndex > firstDragNode && lastDrag <= firstDragNode) {
        console.log("switchb");
    }
    maxDragNode = Math.max(toIndex, maxDragNode);
    minDragNode = Math.min(toIndex, minDragNode);
    if (toIndex != firstDragNode) {
        lastDrag = toIndex;
    }
    // for (var i = 0; i <= Math.abs(toIndex - fromIndex); i++) {
    //     var node = Math.min(fromIndex, toIndex) + i;
    //     if (selectOn) {
    //         select(getObjFromNodeIndex(node));
    //     } else {
    //         deselect(getObjFromNodeIndex(node));
    //     }
    // }
    event.preventDefault();
});

$(".node").on("dragend", function(event) {
    firstDragNode = -1;
})

$(".node").on("dragenter", function(event) {
    if (firstDragNode === -1) return;
    // console.log("dragenter");
    var fromIndex = getNodeIndexFromObj(firstDragNode);
    var toIndex = getNodeIndexFromObj($(event.target));
    // console.log("l " + nodeLeft);
    // console.log("f " + fromIndex);
    // console.log("t " + toIndex);
    if (nodeLeft < Math.min(fromIndex, toIndex) || nodeLeft > Math.max(fromIndex, toIndex)) {
        // deselect(getObjFromNodeIndex(nodeLeft));
    }
})

$(".node").on("dragleave", function(event) {
    deselect($(event.target));
    // console.log("dragleave");
    nodeLeft = getNodeIndexFromObj($(event.target));
})

// populate reservation table
var tr1 = '<tr class="res clickable mdl" ';
var tr2 = '</tr>';
var td1 = '<td class="mdl">';
var td2 = '</td>';
$("#res_table").append(
    tr1 + 'id="res0">' +
    '<td colspan="4" class="mdl">Down</td>' +
    td1 + reservations[0].Nodes.length + td2 +
    tr2
);
for (var i = 1; i < reservations.length; i++) {
    $("#res_table").append(
        tr1 + 'id="res' + i + '">' +
        td1 + reservations[i].Name + td2 +
        td1 + reservations[i].Owner + td2 +
        td1 + reservations[i].Start + td2 +
        td1 + reservations[i].End + td2 +
        td1 + reservations[i].Nodes.length + td2 +
        tr2
    );
}

// reservation selection
$(".res").click(function() {
    deselectGrid();
    toggle($(this));
});

$(".res").hover(function() {
    $(".node").addClass("noshadow");
    var nodes = reservations[getResIndexFromObj($(this))].Nodes;
    for (var i = 0; i < nodes.length; i++) {
        getObjFromNodeIndex(nodes[i]).removeClass("noshadow");
        getObjFromNodeIndex(nodes[i]).addClass("shadow");
    }
});

$(".res").mouseleave(function() {
    $(".node").removeClass("noshadow")
    $(".node").removeClass("shadow")
    var nodes = reservations[getResIndexFromObj($(this))].Nodes;
    for (var i = 0; i < nodes.length; i++) {
        getObjFromNodeIndex(nodes[i]).removeClass("hover");
    }
});

// deselect on outside click
$(document).click(function(event) {
    if (!$(event.target).hasClass("mdl") && !$(event.target).hasClass("successbar")) {
        $("#successbar").removeClass("active");
    }
    if (!$(event.target).hasClass("node") &&
    !$(event.target).hasClass("res") &&
    !$(event.target).hasClass("igorbtn") &&
    !$(event.target).hasClass("mdl") &&
    !$(event.target).hasClass("actionbar") &&
    !$(event.target).hasClass("navbar")
) {
    deselectGrid();
    deselectTable();
}
});

// New reservation modal
$("#newresmodal").on('show.bs.modal', function() {
    newResHideSpec();
    updateNodeListField();
    $("#nrmodalki").click();
    if (selectedNodes.length > 0) {
        $("#nrmodalnodelist").click();
        $("#dashn").val(selectedNodes.length);
    } else {
        $("#nrmodalnumnodes").click();
    }
    updateCommandLine();
});

function newResShowSpec() {
    $("#newresmaintitle").hide();
    $("#newresspectitle").show();
    $("#newresmain").hide();
    $("#newrescancel").hide();
    $("#newresspec").show();
    $("#newresback").show();
    $("#speculate").hide();
    $("#reserve").hide();
}

function newResHideSpec() {
    $("#newresmaintitle").show();
    $("#newresspectitle").hide();
    $("#newresmain").show();
    $("#newrescancel").show();
    $("#newresspec").hide();
    $("#newresback").hide();
    $("#speculate").show();
    $("#reserve").show();
}

$("#newresback").click(function() {
    newResHideSpec();
})

function runSpeculate() {
    for (var i = 0; i < 10; i++) {
        $("#spec_table").children().eq(i).children().eq(0).text(response.Extra[i].Start);
        $("#spec_table").children().eq(i).children().eq(1).text(response.Extra[i].End);
    }
    hideLoaders();
    if (parseResult(false)) {
        newResShowSpec();
    }
}

$(".specreserve").click(function() {
    updateCommandLine();
    $("#dasha").val($(this).parent().prev().prev().html());
})

function runNew() {
    hideLoaders();
    if (parseResult()) {
        $("#newresmodal").modal("hide");
        $(".dash").val("");
    }
}

$(".newresmodalgobtn").click(function(event) {
    updateCommandLine();
    if ($(event.target).hasClass("speculate")) {
        command += " -s";
        showLoader($(this));
        execute(runSpeculate);
        return;
    }
    showLoader($(this));
    execute(runNew);
});

$("#nrmodalki").click(function(event){
    $("#nrmodalki").addClass("active");
    $(".switchki").show();
    $("#nrmodalcobbler").removeClass("active");
    $(".switchcobbler").hide();
    updateCommandLine();
})

$("#nrmodalcobbler").click(function(event){
    $("#nrmodalcobbler").addClass("active");
    $(".switchcobbler").show();
    $("#nrmodalki").removeClass("active");
    $(".switchki").hide();
    updateCommandLine();
})

$("#nrmodalnumnodes").click(function(event){
    $("#nrmodalnumnodes").addClass("active");
    $(".switchnumnodes").show();
    $("#nrmodalnodelist").removeClass("active");
    $(".switchnodelist").hide();
    updateCommandLine();
})

$("#nrmodalnodelist").click(function(event){
    $("#nrmodalnodelist").addClass("active");
    $(".switchnodelist").show();
    $("#nrmodalnumnodes").removeClass("active");
    $(".switchnumnodes").hide();
    updateCommandLine();
})

// Update new reservation command
var command = "";
function updateCommandLine() {
    $(".errorparent").hide();
    if ($("#dashr").val() === "" ||
    ($("#nrmodalki").hasClass("active") ?
    $("#dashk").val() === "" ||
    $("#dashi").val() === "" :
    $("#dashp").val() === ""
) ||
($("#nrmodalnodelist").hasClass("active") ?
$("#dashw").val() === "" :
$("#dashn").val() === ""
)) {
    $(".newresmodalgobtn").prop("disabled", true);
} else {
    $(".newresmodalgobtn").prop("disabled", false);
}

command =
"igor sub " +
"-r " + $("#dashr").val() + " " +
($("#nrmodalki").hasClass("active") ?
"-k " + $("#dashk").val() + " " +
"-i " + $("#dashi").val() + " " :
"-p " + $("#dashp").val() + " "
) +
($("#nrmodalnodelist").hasClass("active") ?
"-w " + cluster + "[" + $("#dashw").val() + "] " :
"-n " + $("#dashn").val() + " "
) +
($("#dashc").val() === "" ? "" : "-c " + $("#dashc").val() + " ") +
($("#dasht").val() === "" ? "" : "-t " + $("#dasht").val() + " ") +
($("#dasha").val() === "" ? "" : "-a " + $("#dasha").val() + " ")
;
$("#commandline").html(command);
}

$(".modal").keyup(function(event) {
    updateCommandLine();
    pUpdateCommandLine();
    eUpdateCommandLine();
});

$("#dashn").click(function(event) {
    updateCommandLine();
});

// Delete reservation modal
function runDelete() {
    hideLoaders();
    if (parseResult()) {
        $("#deleteresmodal").modal("hide");
        getObjFromResIndex(selectedRes).hide();
        deselectGrid();
        deselectTable();
    }
}

$(".deleteresmodalgobtn").click(function() {
    command = "igor del " + reservations[selectedRes].Name;
    $(".errorparent").hide();
    showLoader($(this));
    execute(runDelete);
});

// Power modal
$("#powermodal").on('show.bs.modal', function() {
    if (selectedRes > 0) {
        $("#pdashr").val(reservations[selectedRes].Name);
        $("#pmodalres").click();
    } else {
        $("#pdashr").val("");
        $("#pmodalnodelist").click();
    }
    updateNodeListField("pdashn");
    pUpdateCommandLine();
});

function runPower() {
    hideLoaders();
    if (parseResult()) {
        $("#powermodal").modal("hide");
        $(".pdash").val("");
        deselectGrid();
        deselectTable();
    }
}

$(".powermodalgobtn").click(function(event) {
    pUpdateCommandLine();
    showLoader($(this));
    command += $(this).attr("id");
    execute(runPower);
})

$("#pmodalres").click(function(event){
    $("#pmodalres").addClass("active");
    $("#pdashrfg").show();
    $("#pmodalnodelist").removeClass("active");
    $("#pdashnfg").hide();
    pUpdateCommandLine();
})

$("#pmodalnodelist").click(function(event){
    $("#pmodalnodelist").addClass("active");
    $("#pdashnfg").show();
    $("#pmodalres").removeClass("active");
    $("#pdashrfg").hide();
    pUpdateCommandLine();
})

// Update power command
var command = "";
function pUpdateCommandLine() {
    $(".errorparent").hide();
    if ($("#pmodalres").hasClass("active") ?
    $("#pdashr").val() === "" :
    $("#pdashn").val() === "") {
        $(".powermodalgobtn").prop("disabled", true);
    } else {
        $(".powermodalgobtn").prop("disabled", false);
    }

    command =
    "igor power " +
    ($("#pmodalres").hasClass("active") ?
        "-r " + $("#pdashr").val() + " " :
        "-n " + cluster + "[" + $("#pdashn").val() + "] "
    )
    $("#pcommandline").html(command);
}

// Extend modal
$("#extendmodal").on('show.bs.modal', function() {
    if (selectedRes > 0) {
        $("#edashr").val(reservations[selectedRes].Name);
    } else {
        $("#edashr").val("");
    }
    eUpdateCommandLine();
});

function runExtend() {
    hideLoaders();
    if(parseResult()) {
        $("#extendmodal").modal("hide");
        $(".edash").val("");
        deselectGrid();
        deselectTable();
    }
}

$(".extendmodalgobtn").click(function(event) {
    eUpdateCommandLine();
    showLoader($(this));
    execute(runExtend);
});

// Update extend command
var command = "";
function eUpdateCommandLine() {
    $(".errorparent").hide();
    if ($("#edashr").val() === "") {
        $(".extendmodalgobtn").prop("disabled", true);
    } else {
        $(".extendmodalgobtn").prop("disabled", false);
    }

    command =
    "igor extend " +
    "-r " + $("#edashr").val() + " " +
    ($("#edasht").val() == "" ?
    "" : "-t " + $("#edasht").val()
)
;
$("#ecommandline").html(command);
}
