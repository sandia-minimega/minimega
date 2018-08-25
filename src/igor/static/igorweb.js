var response = "";

// selection functions
function getResIndexFromObj(obj) {
    return parseInt((obj.attr("id")).slice(3));
}

function getObjFromResIndex(index) {
    return $("#res" + index);
}

function getResIndexByName(name) {
    if (name === "") {
        return -1;
    }
    for (var i = 0; i < reservations.length; i++) {
        if (reservations[i].Name === name) {
            return i;
        }
    }
    return -1;
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
    $(".responseparent").hide();
    $("#deletemodaldialog").addClass("modal-sm");
    response = "";
    $(".command").html(command);
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

function getReservations() {
    showBigLoaders();
    var curResName = "";
    if (selectedRes != -1) {
        curResName = reservations[selectedRes].Name;
    }
    var selectedNodestmp = selectedNodes;
    deselectGrid();
    deselectTable();
    $.get(
        "/run/",
        {run: "igor show"},
        function(data) {
            setTimeout(function() {
                var rsp = JSON.parse(data);
                reservations = rsp.Extra;
                console.log(reservations)
                showReservationData();
                if (newResName !== "" && response.Success) {
                    selectedRes = getResIndexByName(newResName);
                    newResName = "";
                } else {
                    selectedRes = getResIndexByName(curResName);
                }
                if (selectedRes != -1) {
                    select(getObjFromResIndex(selectedRes));
                } else {
                    for (var i = 0; i < selectedNodestmp.length; i++) {
                        select(getObjFromNodeIndex(selectedNodestmp[i]));
                    }
                }
                hideBigLoaders();
            }, 700)
        }
    );
}

function parseResult() {
    $(".response").html(response.Message);
    if (response.Success) {
        $(".responseparent").addClass("success");
    } else {
        $(".responseparent").removeClass("success");
    }
    $("#deletemodaldialog").removeClass("modal-sm");
    $(".responseparent").show();
    return response.Success;
}

function showLoader(obj) {
    obj.children().eq(0).css("visibility", "hidden");
    obj.children().eq(1).css("visibility", "visible");
    $(".dash, .edash, .pdash").prop("disabled", true);
    $(".igorbtn").prop("disabled", true);
    $(".cancel").prop("disabled", true);
}

function hideLoaders() {
    getReservations();
    $(".loader").css("visibility", "hidden");
    $(".mdlcmdtext").css("visibility", "visible");
    $(".dash, .edash, .pdash").prop("disabled", false);
    $(".igorbtn").prop("disabled", false);
    $(".cancel").prop("disabled", false);
}

function showBigLoaders() {
    $(".bigloader").show();
    $(".node, #table").animate({"opacity": 0}, 700, function() {});
    $("#nodegrid").css("min-height", "864px");
    $("#table").parent().css("min-height", "400px");
}

function hideBigLoaders() {
    $(".bigloader").hide();
    $(".node, #table").animate({"opacity": 1}, 700, function() {});
    $("#nodegrid").css("min-height", "auto");
    $("#table").parent().css("min-height", "auto");
}

var selectOn = true;
var firstDragNode = -1;
var nodes = document.getElementsByClassName("node");
var nodeLeft;
var maxDragNode;
var minDragNode;
var extremeMax;
var extremeMin;
var minDragNode;
var lastDrag;
function showReservationData(){
    $("#nodegrid").html('<div class="mdl bigloader"></div>');
    $("#res_table").html("");
    // populate node grid
    var newcol = '<div class="col" style="padding: 0">' +
    '    <div class="list-group" ';
    for (var i = 0; i < rackWidth; i++) {
        $('#nodegrid').append(newcol + 'id="col' + i + '"></div></div>');
    }
    var grid = '<div draggable="true" tabIndex="-1" style="opacity: 0; width:100%; padding: 12px; padding-left: 0px; padding-right: 0px; cursor: pointer;" ';
    for (var i = startNode; i <= endNode; i++) {
        col = (i - 1) % rackWidth;
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
    hideBigLoaders();

    // node grid selections
    $(".node").click(function(event) {
        deselectTable();
        toggle($(this));
    });

    // node hover to cause res table hover
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

    // node dragging
    for (var i = 0; i < nodes.length; i++) {
        nodes[i].addEventListener("dragstart", function(event) {
            deselectTable();
            firstDragNode = $(event.target);
            maxDragNode = getNodeIndexFromObj(firstDragNode);
            minDragNode = maxDragNode;
            extremeMax = maxDragNode;
            extremeMin = maxDragNode;
            lastDrag = maxDragNode;
            if ($(event.target).hasClass("active")) {
                selectOn = false;
            } else {
                selectOn = true;
            }
            toggle($(event.target));
            var crt = this.cloneNode(true);
            crt.style.display = "none";
            document.body.appendChild(crt);
            event.dataTransfer.setDragImage(crt, 0, 0);
            event.dataTransfer.setData('text/plain', '');
        }, false);
    }

    $(".node").on("dragover", function(event) {
        if (firstDragNode === -1) return;
        var fromIndex = getNodeIndexFromObj(firstDragNode);
        var toIndex = getNodeIndexFromObj($(event.target));
        if (selectOn) {
            if (toIndex > minDragNode && toIndex < maxDragNode) {
                if (toIndex < fromIndex) {
                    selectNodes(minDragNode, toIndex, !selectOn);
                    minDragNode = toIndex;
                }
                if (toIndex > fromIndex) {
                    selectNodes(maxDragNode, toIndex, !selectOn);
                    maxDragNode = toIndex;
                }
            } else {
                selectNodes(fromIndex, toIndex, selectOn);
            }
            if (toIndex < fromIndex && lastDrag >= fromIndex) {
                maxDragNode = fromIndex;
                if (extremeMax !== fromIndex) {
                    selectNodes(extremeMax, fromIndex + 1, !selectOn);
                }
            }
            if (toIndex > fromIndex && lastDrag <= fromIndex) {
                minDragNode = fromIndex;
                if (extremeMin !== fromIndex) {
                    selectNodes(extremeMin, fromIndex - 1, !selectOn);
                }
            }
            maxDragNode = Math.max(toIndex, maxDragNode);
            minDragNode = Math.min(toIndex, minDragNode);
            extremeMax = Math.max(extremeMax, maxDragNode);
            extremeMin = Math.min(extremeMin, minDragNode);
            if (toIndex != firstDragNode) {
                lastDrag = toIndex;
            }
            event.preventDefault();
        } else {
            selectNodes(fromIndex, toIndex, false);
        }
    });

    $(".node").on("dragend", function(event) {
        firstDragNode = -1;
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
}
showReservationData();

// heartbeat
setInterval(function() {
    getReservations();
}, 60000);

// deselect on outside click
$(document).click(function(event) {
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
    if (response.Success) {
        newResShowSpec();
    } else {
        parseResult();
    }
}

$(".specreserve").click(function() {
    updateCommandLine();
    $("#dasha").val($(this).parent().prev().prev().html());
})

function runNew() {
    hideLoaders();
    parseResult();
}

var newResName = "";
$(".newresmodalgobtn").click(function(event) {
    updateCommandLine();
    if ($(event.target).hasClass("speculate")) {
        command += " -s";
        showLoader($(this));
        execute(runSpeculate);
        return;
    }
    showLoader($(this));
    newResName = $("#dashr").val();
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
"igor sub" +
" -r " + $("#dashr").val() +
($("#nrmodalki").hasClass("active") ?
" -k " + $("#dashk").val() +
" -i " + $("#dashi").val() :
" -p " + $("#dashp").val()
) +
($("#nrmodalnodelist").hasClass("active") ?
" -w " + cluster + "[" + $("#dashw").val().replace(/ /g, "") + "]" :
" -n " + $("#dashn").val()
) +
($("#dashc").val() === "" ? "" : " -c " + $("#dashc").val()) +
($("#dasht").val() === "" ? "" : " -t " + $("#dasht").val()) +
($("#dasha").val() === "" ? "" : " -a " + $("#dasha").val())
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
    deselectGrid();
    deselectTable();
    hideLoaders();
    if (!response.Success) {
        parseResult();
    } else {
        $("#deleteresmodal").modal("hide");
    }
}

$(".deleteresmodalgobtn").click(function() {
    command = "igor del " + reservations[selectedRes].Name;
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
    parseResult();
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
        "-n " + cluster + "[" + $("#pdashn").val().replace(/ /g, "") + "] "
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
    parseResult();
}

$(".extendmodalgobtn").click(function(event) {
    eUpdateCommandLine();
    showLoader($(this));
    execute(runExtend);
});

// Update extend command
var command = "";
function eUpdateCommandLine() {
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
    );
    $("#ecommandline").html(command);
}

// copy response
$(".copy").click(function(event) {
    var textArea = document.createElement("textarea");
    textArea.value = $(event.target).parent().parent().parent().find("code").html();
    document.body.appendChild(textArea);
    textArea.select();
    document.execCommand("copy");
    textArea.remove();
    $(".copytooltip").show();
    $(".copytooltip").animate({"opacity": 0.95}, 250, function() {
        setTimeout(function() {
            $(".copytooltip").animate({"opacity": 0}, 250, function() {
                $(".copytooltip").hide();
            })
        }, 1000);
    });
});

$(".igorbtn").click(function() {
    $(".responseparent").hide();
    $("#deletemodaldialog").addClass("modal-sm");
});
