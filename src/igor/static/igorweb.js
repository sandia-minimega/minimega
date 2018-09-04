/*****************
 GLOBAL VARIABLES

 *****************/

// holds the last response from web.go
var response = "";
// stores whether the current drag event is an on or off selection
// (whether the selection started with a selected or unselected node,
//      to decide whether to select or deselect the nodes dragged over)
var selectOn = true;
// index of where the drag started
var firstDragNode = -1;
// array of all of the nodes
var nodes = document.getElementsByClassName("node");
// drag can go back and forth, so max and min drag node record
//      the first and last nodes of the current selection from the drag, while
//      extreme min and max record the smallest and largest node number reached
//          by the current drag, whether or not currently selected
// maximum node number that is selected in the current drag
var maxDragNode;
// minimum node number that is selected in the current drag
var minDragNode;
// maximum node number that was reached in current drag
var extremeMax;
// minimum node number reached in the current drag
var extremeMin;
// saves the last node passed through in the current
var lastDrag;



/*************************
 NODE TO OBJECT FUNCTIONS

 *************************/

// input a res object (row from res table)
//      returns its index in the reservations array
function getResIndexFromObj(obj) {
    return parseInt((obj.attr("id")).slice(3));
}

// input the index of a reservation in the reservations array
//      returns the respective object
function getObjFromResIndex(index) {
    return $("#res" + index);
}

// input the name of a reservation
//      returns the respective index in the reservations array
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

// input a node object
//      returns its node number
function getNodeIndexFromObj(obj) {
    return parseInt(obj.attr("id"));
}

// input a node number
//      returns the object of that node
function getObjFromNodeIndex(index) {
    return $("#" + index);
}



/*************************************
 NODE/RESERVATION SELECTION FUNCTIONS

 *************************************/

 // array of the currently selected nodes, kept in increasing order
 //      to create ranges on commands
 var selectedNodes = [];
 // currently selected reservation in the reservation table:
 //      its index in the reservations array
 //      (only one reservation can be selected at a time)
 //      -1 if none selected
 var selectedRes = -1;
 // whether the actions on reservations are shown (delete and extend)
 //      if not shown, then only power-cycle will appear
 var resActionShown = false;

// select a reservation or node(s)
//      will do nothing if already selected
//      called on an object representing a single reservation, or
//          one or several nodes
// to call this, use JQuery
//      for a reservation: select($("#res[RES_INDEX]"))
//      for a node: select($("#[NODE_NUMBER]"))
//      for several nodes: select($(".node.[CLASS]"))
function select(obj) {
    if (obj.length === 0) return;
    if (obj.hasClass("active")) return;
    // if node
    if (obj.hasClass("node")) {
        // add all of the nodes (or one) to the selectedNodes array
        obj.each(function () {
            if ($(this).hasClass("active")) return;
            if (selectedNodes.includes(getNodeIndexFromObj($(this)))) return;
            selectedNodes.push(getNodeIndexFromObj($(this)));
        });
        selectedNodes.sort((a, b) => a - b);
    // if reservation
    } else {
        var selectedResTemp = getResIndexFromObj(obj);
        // deselect res table before selecting the new reservation
        //      hide res action bar only if reservation just selected is down
        //      (see checkHideActionBar())
        deselectTable((selectedResTemp === 0) ? 0 : -2);
        selectedRes = selectedResTemp;
        // select all nodes that are in the reservation
        //      all nodes will be deselected by now, so toggle works
        toggleNodes(reservations[selectedRes].Nodes);
    }
    obj.addClass("active");
    showActionBar();
}

// deselect a reservation or node(s)
//      no action if already deselected
//      updates selectedRes/selectedNodes
// call using JQuery, similar to select() above ^
function deselect(obj) {
    if (!obj.hasClass("active")) return;
    obj.removeClass("active");
    // if node
    if (obj.hasClass("node")) {
        // remove all nodes in object from selectNodes array
        obj.each(function () {
            selectedNodes.splice(selectedNodes.indexOf(getNodeIndexFromObj($(this))), 1);
        });
    // if reservation
    } else {
        selectedRes = -1;
    }
    checkHideActionBar();
}

// toggle a list of nodes between selected and unselected, separately
function toggleNodes(nodes) {
    for (var i = 0; i < nodes.length; i++) {
        toggle(getObjFromNodeIndex(nodes[i]));
    }
}

// toggle a specific object (reservation or node)
//      between selected and unselected
function toggle(obj) {
    if (obj.hasClass("active")) {
        deselect(obj);
        return true;
    } else {
        select(obj);
        return false;
    }
}

// select or deselect a range of nodes
//      range is startNode to endNode, inclusive
//      toOn determines whether to change to
//          selected (true) or unselected (false)
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

// deselect the entire grid of nodes
function deselectGrid() {
    $(".node").removeClass("active");
    selectedNodes = [];
    checkHideActionBar()
}

// deselect the entire reservation table
//      change selectedRes to given index
//      hides action bar if new selected index is 0 (down)
function deselectTable(sr = -1) {
    $(".res").removeClass("active");
    selectedRes = sr;
    checkHideActionBar()
}

// show the bar at the bottom of the screen with actions to carry on selections
//      if only nodes are selected, only power-cycle is shown
//      if a reservation is selected, delete and extend are also shown
function showActionBar() {
    if (selectedRes > 0) {
        showResActions();
    } else {
        hideResActions();
    }
    $("#actionbar").addClass("active");
}

// hide the action bar if only nodes are selected, and
//      take care of res actions
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

// hide the action bar
function hideActionBar() {
    $("#actionbar").removeClass("active");
}

// show the reservation actions (delete and extend)
function showResActions() {
    $(".resaction").fadeIn(200, function() {
        $(".resaction").show();
    });
}

// hide the reservation actions (delete and extend) but leave power-cycle
// generally used if going from reservation selection to just node selection
function hideResActions() {
    $(".resaction").fadeOut(200, function() {
        $(".resaction").hide();
    });
}

// deselect all on outside click
$(document).click(function(event) {
    if (!$(event.target).hasClass("node") &&
        !$(event.target).hasClass("res") &&
        !$(event.target).hasClass("igorbtn") &&
        !$(event.target).hasClass("mdl") &&
        !$(event.target).hasClass("actionbar") &&
        !$(event.target).hasClass("navbar")) {
        deselectGrid();
        deselectTable();
    }
});



/********************
 EXECUTION FUNCTIONS

 ********************/

// send a command to web.go for execution, stored in the "command" variable
//      when response is received, the function onResponse is called,
//      which can use "response" to decide how to proceed
function execute(onResponse) {
    $(".responseparent").hide();
    $("#deletemodaldialog").addClass("modal-sm");
    response = "";
    $(".command").html(command);
    $.get(
        "/run/",
        {run: command},
        function(data) {
            response = JSON.parse(data);
            onResponse();
        }
    );
}

// show response from web.go at the bottom of the currently open modal,
//      with green background on success and red on failure
//      generally called in the onResponse function passed into execute()
//      updates on screen reservations information on success
function parseResult() {
    $(".response").html(response.Message);
    if (response.Success) {
        $(".responseparent").addClass("success");
        getReservations();
    } else {
        $(".responseparent").removeClass("success");
    }
    $("#deletemodaldialog").removeClass("modal-sm");
    $(".responseparent").show();
    return response.Success;
}

// when any button is clicked, hide the response from server
$(".igorbtn").click(function() {
    $(".responseparent").hide();
    $("#deletemodaldialog").addClass("modal-sm");
});

// send a request to web.go to issue an "igor show" command to update the
//      reservation information
function getReservations() {
    $.get(
        "/run/",
        {run: "igor show"},
        function(data) {
            // save current selection information so it can be reselected when
            //      html is regenerated
            var curResName = "";
            if (selectedRes != -1) {
                curResName = reservations[selectedRes].Name;
            }
            var selectedNodestmp = selectedNodes;
            // update reservations array, but don't change "response" because
            //      this isn't considered a command
            var rsp = JSON.parse(data);
            reservations = rsp.Extra;
            showReservationData();
            // if a new reservation was just created (successfully),
            //      then select it
            if (newResName !== "" && response.Success) {
                selectedRes = getResIndexByName(newResName);
                newResName = "";
            } else {
                selectedRes = getResIndexByName(curResName);
            }
            // otherwise select what was already selected
            if (selectedRes != -1) {
                select(getObjFromResIndex(selectedRes));
            } else {
                for (var i = 0; i < selectedNodestmp.length; i++) {
                    select(getObjFromNodeIndex(selectedNodestmp[i]));
                }
            }
        }
    );
}

// heartbeat: update reservations every set time
var heartrate = 10000; // every 10 seconds
var heartbeat = setInterval(function() {
    getReservations();
}, heartrate);

// stop heartbeat when user clicks outside page
$(window).blur(function() {
    clearInterval(heartbeat);
    heartbeat = 0;
});

// getReservations and restart heartbeat when user returns to page
// (a.k.a. perform CPR)
$(window).focus(function() {
    if (!heartbeat) {
        getReservations();
        heartbeat = setInterval(function() {
            getReservations();
        }, heartrate);
    }
});

// show a loading circle on the the button clicked,
//      given an object referring to what was clicked
// disables all other buttons to force user to wait until the action completes,
//      and make it clear that the command is being sent for execution
// generally called when an action button is clicked on a modal,
//      until a response is received from web.go
// to finish the loading, call hideLoaders()
function showLoader(obj) {
    // hide text
    obj.children().eq(0).css("visibility", "hidden");
    // show loader circle
    obj.children().eq(1).css("visibility", "visible");
    // disable all buttons
    $(".dash, .edash, .pdash").prop("disabled", true);
    $(".igorbtn").prop("disabled", true);
    $(".cancel").prop("disabled", true);
}

// return the application to the user when loading completes
// generally called when a response is received from web.go and the
//      application is currently in loading state due to showLoader
function hideLoaders() {
    $(".loader").css("visibility", "hidden");
    $(".mdlcmdtext").css("visibility", "visible");
    $(".dash, .edash, .pdash").prop("disabled", false);
    $(".igorbtn").prop("disabled", false);
    $(".cancel").prop("disabled", false);
}

// update the node list field in the current command to the currently selected
//      nodes
// default is the new reservation node list field (-w),
//      specify a different element id if needed
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



/****************************************
 GENERATE NODE GRID AND RESERVATION TABLE

 ****************************************/

// display new reservation information on node grid and reservation table,
//      by clearing them and regenerating the html
// NOTE: this is usually so quick it won't even be visible if the data is
//          the same,
//          but every so often it will cause a user's clicks to be unregistered
function showReservationData(){
    // sort reservations based on:
    //      start time, then
    //      number of nodes, then
    //      reservation name
    reservations.sort(function (a, b) {
        if (a.Name === "") return -1;
        if (b.Name === "") return 1;
        var diff = a.StartInt - b.StartInt;
        if (diff === 0) {
            var diff = a.Nodes.length - b.Nodes.length;
            if (diff === 0) {
                if (a.Name > b.Name) {
                    diff = 1;
                } else {
                    diff = -1;
                }
            }
        }
        return diff;
    });
    $("#nodegrid").html("");
    $("#res_table").html("");



    // POPULATE NODE GRID

    var newcol = '<div class="col" style="padding: 0">' +
    '    <div class="list-group" ';
    for (var i = 0; i < rackWidth; i++) {
        $('#nodegrid').append(newcol + 'id="col' + i + '"></div></div>');
    }
    var grid = '<div draggable="true" tabIndex="-1" style="opacity: 1; width:100%; padding: 12px; padding-left: 0px; padding-right: 0px; cursor: pointer;" ';
    for (var i = startNode; i <= endNode; i++) {
        col = (i - 1) % rackWidth;
        var classes = ' class="list-group-item list-group-item-action node up available unselected" ';
        $("#col" + col).append(grid + classes + ' id="' + i +'">' + i + '</div>');
    }

    // mark all nodes that are down
    for (var i = 0; i < reservations[0].Nodes.length; i++) {
        getObjFromNodeIndex(reservations[0].Nodes[i]).removeClass("up");
        getObjFromNodeIndex(reservations[0].Nodes[i]).addClass("down");
    }

    // mark all nodes that are reserved
    for (var j = 1; j < reservations.length; j++) {
        for (var i = 0; i < reservations[j].Nodes.length; i++) {
            getObjFromNodeIndex(reservations[j].Nodes[i]).removeClass("available");
            getObjFromNodeIndex(reservations[j].Nodes[i]).addClass("reserved");
        }
    }

    // select/deselect node on click
    $(".node").click(function(event) {
        deselectTable();
        toggle($(this));
    });

    // node hover to cause:
    //      reservations that have this reservation to hover
    //      color of node to hover in the key
    $(".node").hover(function() {
        var node = getNodeIndexFromObj($(this));
        for (var i = 0; i < reservations.length; i++) {
            if (reservations[i].Nodes.includes(node)) {
                getObjFromResIndex(i).addClass("hover");
            };
        }
        if ($(this).hasClass("available")) {
            $(".key.available.headtext").addClass("hover");
        }
        if ($(this).hasClass("reserved")) {
            $(".key.reserved.headtext").addClass("hover");
        }
        if ($(this).hasClass("up")) {
            $(".key.up.headtext").addClass("hover");
        }
        if ($(this).hasClass("down")) {
            $(".key.down.headtext").addClass("hover");
        }
        if ($(this).hasClass("available")) {
            if ($(this).hasClass("up")) {
                $(".key.available.up").addClass("hover");
            } else {
                $(".key.available.down").addClass("hover");
            }
        } else if ($(this).hasClass("reserved")) {
            if ($(this).hasClass("up")) {
                $(".key.reserved.up").addClass("hover");
            } else {
                $(".key.reserved.down").addClass("hover");
            }
        }
    });

    // remove res and key hover on exit
    $(".node").mouseleave(function() {
        $(".res, .key").removeClass("hover");
    });



    // NODE DRAGGING
    // (see global variables)

    // record drag information when it begins
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
            // roundabout way to prevent "ghost" element during drag
            var crt = this.cloneNode(true);
            crt.style.display = "none";
            document.body.appendChild(crt);
            event.dataTransfer.setDragImage(crt, 0, 0);
            event.dataTransfer.setData('text/plain', '');
        }, false);
    }

    // toggle nodes when drag passes over
    $(".node").on("dragover", function(event) {
        if (firstDragNode === -1) return;
        var fromIndex = getNodeIndexFromObj(firstDragNode);
        var toIndex = getNodeIndexFromObj($(event.target));
        // behavior is different based on whether node dragged
        //      was already selected
        // if node dragged was unselected:
        //      selection from drag can be reversed by going back the other way
        //      selection under drag that is reversed is lost, no memory
        if (selectOn) {
            // decrease drag selection if drag returns towards first node
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
            // detect when drag passes over first node in the other direction
            //      to deselect all nodes in original direction
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
            // update dragging globals
            maxDragNode = Math.max(toIndex, maxDragNode);
            minDragNode = Math.min(toIndex, minDragNode);
            extremeMax = Math.max(extremeMax, maxDragNode);
            extremeMin = Math.min(extremeMin, minDragNode);
            if (toIndex != firstDragNode) {
                lastDrag = toIndex;
            }
            event.preventDefault();
        // if node dragged was selected:
        //      deselect all nodes that drag passes over, and
        //      drag cannot be undone
        } else {
            selectNodes(fromIndex, toIndex, false);
        }
    });

    // on the end of the drag
    $(".node").on("dragend", function(event) {
        firstDragNode = -1;
    })



    // POPULATE RESERVATION TABLE

    var tr1 = '<tr class="res clickable mdl ';
    var tr2 = '</tr>';
    var td1 = '<td class="mdl">';
    var tdcurrent = '<td class="mdl current">';
    var td2 = '</td>';
    for (var i = 1; i < reservations.length; i++) {
        if (reservations[i].StartInt < reservations[0].StartInt) {
            $("#res_table").append(
                tr1 + classes + 'id="res' + i + '">' +
                td1 + reservations[i].Name + td2 +
                td1 + reservations[i].Owner + td2 +
                tdcurrent + reservations[i].Start + td2 +
                td1 + reservations[i].End + td2 +
                td1 + reservations[i].Nodes.length + td2 +
                tr2
            );
        } else {
            $("#res_table").append(
                tr1 + classes + 'id="res' + i + '">' +
                td1 + reservations[i].Name + td2 +
                td1 + reservations[i].Owner + td2 +
                td1 + reservations[i].Start + td2 +
                td1 + reservations[i].End + td2 +
                td1 + reservations[i].Nodes.length + td2 +
                tr2
            );
        }
    }

    // reservation selection on click
    $(".res").click(function() {
        deselectGrid();
        toggle($(this));
    });

    // when hovering over a reservation:
    //      hover the nodes belonging to the reservation, and
    //      hover the type of nodes in the key
    $(".res").hover(function() {
        // remove shadows from all nodes temporarily to create contrast
        $(".node").addClass("noshadow");
        var resNodes = reservations[getResIndexFromObj($(this))].Nodes;
        var up = false;
        var down = false;
        for (var i = 0; i < resNodes.length; i++) {
            // key hover
            if (reservations[0].Nodes.includes(resNodes[i])) {
                down = true;
            } else {
                up = true;
            }
            $(".key.reserved.headtext").addClass("hover");
            if (down) {
                $(".key.down.headtext").addClass("hover");
                $(".key.reserved.down").addClass("hover");
            }
            if (up) {
                $(".key.up.headtext").addClass("hover");
                $(".key.reserved.up").addClass("hover");
            }
            // node hover
            getObjFromNodeIndex(resNodes[i]).removeClass("noshadow");
            getObjFromNodeIndex(resNodes[i]).addClass("shadow");
        }
    });

    // remove hover of nodes and key when res hover ends
    $(".res").mouseleave(function() {
        $(".key").removeClass("hover");
        $(".node").removeClass("noshadow");
        $(".node").removeClass("shadow");
    });
}

// show reservations when page first loads
showReservationData();



/*********************
 NEW RESERVATION MODAL

 *********************/

// open a new reservation modal
$("#newbutton").click(function() {
    newResHideSpec();
    updateNodeListField();
    $("#nrmodalki").click();
    // if nodes are selected, set fields and go to node list
    if (selectedNodes.length > 0) {
        $("#nrmodalnodelist").click();
        $("#dashn").val(selectedNodes.length);
    // otherwise go to number of nodes
    } else {
        $("#nrmodalnumnodes").click();
    }
    updateCommandLine();
    $("#newresmodal").modal("show");
});

// go to speculate page in new reservation modal,
//      when Speculate is clicked
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

// return to main new reservation page in modal,
//      when Back is clicked when in Speculate
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

// go back to main page of new reservation modal when Back is clicked
$("#newresback").click(function() {
    newResHideSpec();
})

// to be run when a speculate command returns
// display speculate data (on success) or show error (on failure)
function onSpeculateReturn() {
    if (response.Success) {
        for (var i = 0; i < 10; i++) {
            $("#spec_table").children().eq(i).children().eq(0).text(response.Extra[i].Start);
            $("#spec_table").children().eq(i).children().eq(1).text(response.Extra[i].End);
        }
        newResShowSpec();
    } else {
        parseResult();
    }
    hideLoaders();
}

// when Reserve is clicked in Speculate page
// an "igor sub" command is issued with all of the same fields as the previous
//      page but with the -a tag set to the selected date
$(".specreserve").click(function() {
    showLoader($(this));
    var i = $(this).parent().parent().index();
    $("#dasha").val(response.Extra[i].Formatted);
    updateCommandLine();
    newResName = $("#dashr").val();
    execute(onNewReturn);
})

// to be run when new reservation command returns from web.go
function onNewReturn() {
    hideLoaders();
    if ($("#newresspec").is(":visible") && response.Success) {
        newResHideSpec();
    }
    parseResult();
}

// run a new reservation command
var newResName = "";
$(".newresmodalgobtn").click(function(event) {
    updateCommandLine();
    // if speculate, then run Speculate instead and go to Speculat page
    if ($(event.target).hasClass("speculate")) {
        command += " -s";
        showLoader($(this));
        execute(onSpeculateReturn);
        return;
    }
    showLoader($(this));
    // save name of new reservation to select it if command is successful
    newResName = $("#dashr").val();
    execute(onNewReturn);
});

// switch between -k -i fields and -p field
// if ki side is clicked, switch to those fields
$("#nrmodalki").click(function(event){
    $("#nrmodalki").addClass("active");
    $(".switchki").show();
    $("#nrmodalcobbler").removeClass("active");
    $(".switchcobbler").hide();
    updateCommandLine();
})

// if p side is clicked, switch to cobbler field
$("#nrmodalcobbler").click(function(event){
    $("#nrmodalcobbler").addClass("active");
    $(".switchcobbler").show();
    $("#nrmodalki").removeClass("active");
    $(".switchki").hide();
    updateCommandLine();
})

// switch between number of nodes and node list
// if left side is clicked, switch to number of nodes (default)
$("#nrmodalnumnodes").click(function(event){
    $("#nrmodalnumnodes").addClass("active");
    $(".switchnumnodes").show();
    $("#nrmodalnodelist").removeClass("active");
    $(".switchnodelist").hide();
    updateCommandLine();
})

// if right side is clicked, switch to node list
$("#nrmodalnodelist").click(function(event){
    $("#nrmodalnodelist").addClass("active");
    $(".switchnodelist").show();
    $("#nrmodalnumnodes").removeClass("active");
    $(".switchnumnodes").hide();
    updateCommandLine();
})

// update the command variable holding the new reservation command
var command = "";
function updateCommandLine() {
    // only enable Speculate and Reserve if all required fields are nonempty
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

    // compile a command string based on state of switches and whether optional
    //      fields are nonempty
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

// whenever a key is pressed in any modal, update the command string
$(".modal").keyup(function(event) {
    updateCommandLine();
    pUpdateCommandLine();
    eUpdateCommandLine();
});

// update the command line on click of number of nodes field, because
//      it can be changed with the arrows that some browsers place in
//      numerical fields
$("#dashn").click(function(event) {
    updateCommandLine();
});



/************************
 DELETE RESERVATION MODAL

 ************************/

// to be called when a delete command returns
function onDeleteReturn() {
    deselectGrid();
    deselectTable();
    hideLoaders();
    if (!response.Success) {
        parseResult();
    } else {
        getReservations();
        $("#deleteresmodal").modal("hide");
    }
}

// delete the selected reservation when Delete is clicked
$(".deleteresmodalgobtn").click(function() {
    command = "igor del " + reservations[selectedRes].Name;
    showLoader($(this));
    execute(onDeleteReturn);
});



/*****************
 POWER-CYCLE MODAL

 *****************/

// on show modal either use reservation or node list field
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

// to run when a power command returns
function onPowerReturn() {
    hideLoaders();
    parseResult();
}

// run a power command when an action button is clicked
$(".powermodalgobtn").click(function(event) {
    pUpdateCommandLine();
    showLoader($(this));
    command += $(this).attr("id");
    execute(onPowerReturn);
})

// switch between reservation and node list fields
// switch to reservation field
$("#pmodalres").click(function(event){
    $("#pmodalres").addClass("active");
    $("#pdashrfg").show();
    $("#pmodalnodelist").removeClass("active");
    $("#pdashnfg").hide();
    pUpdateCommandLine();
})

// switch to node list field
$("#pmodalnodelist").click(function(event){
    $("#pmodalnodelist").addClass("active");
    $("#pdashnfg").show();
    $("#pmodalres").removeClass("active");
    $("#pdashrfg").hide();
    pUpdateCommandLine();
})

// update the command string for power
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
            // remove spaces per style required in Go files
            "-n " + cluster + "[" + $("#pdashn").val().replace(/ /g, "") + "] "
        )
        $("#pcommandline").html(command);
}



/************
 EXTEND MODAL

 ************/

// on opening modal
$("#extendmodal").on('show.bs.modal', function() {
    if (selectedRes > 0) {
        // set reservation field to selected reservation
        $("#edashr").val(reservations[selectedRes].Name);
    } else {
        $("#edashr").val("");
    }
    eUpdateCommandLine();
});

// to be run when an extend command returns
function onExtendReturn() {
    hideLoaders();
    parseResult();
}

// run extend command when Extend is clicked
$(".extendmodalgobtn").click(function(event) {
    eUpdateCommandLine();
    showLoader($(this));
    execute(onExtendReturn);
});

// update extend command string
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



/*************************
 COPY RESPONSE FROM SERVER

 *************************/

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



/****
 KEY

 ****/

// when any cell is clicked, select the respective nodes
$(".key").click(function(event) {
    deselectGrid();
    deselectTable();
    var obj;
    if ($(this).hasClass("available")) {
        if ($(this).hasClass("up")) {
            obj = $(".node.available.up");
        } else if ($(this).hasClass("down")) {
            obj = $(".node.available.down");
        } else {
            obj = $(".node.available");
        }
    } else if ($(this).hasClass("reserved")) {
        if ($(this).hasClass("up")) {
            obj = $(".node.reserved.up");
        } else if ($(this).hasClass("down")) {
            obj = $(".node.reserved.down");
        } else {
            obj = $(".node.reserved");
        }
    } else if ($(this).hasClass("up")) {
        obj = $(".node.up");
    } else if ($(this).hasClass("down")) {
        obj = $(".node.down");
    }
    select(obj);
});

// when hovering over any cell in key, hover the respective nodes
// hover reservations also, but this is only relevant for some table cells
$(".key").hover(function() {
    $(".node").addClass("noshadow");
    var obj;
    if ($(this).hasClass("available")) {
        if ($(this).hasClass("up")) {
            obj = $(".node.available.up");
        } else if ($(this).hasClass("down")) {
            obj = $(".node.available.down");
        } else {
            obj = $(".node.available");
        }
    } else if ($(this).hasClass("reserved")) {
        if ($(this).hasClass("up")) {
            obj = $(".node.reserved.up");
            for (var i = 1; i < reservations.length; i++) {
                for (var j = 0; j < reservations[i].Nodes.length; j++) {
                    if (!reservations[0].Nodes.includes(reservations[i].Nodes[j])) {
                        getObjFromResIndex(i).addClass("hover");
                        break;
                    }
                }
            }
        } else if ($(this).hasClass("down")) {
            obj = $(".node.reserved.down");
            for (var i = 1; i < reservations.length; i++) {
                for (var j = 0; j < reservations[i].Nodes.length; j++) {
                    if (reservations[0].Nodes.includes(reservations[i].Nodes[j])) {
                        getObjFromResIndex(i).addClass("hover");
                        break;
                    }
                }
            }
        } else {
            obj = $(".node.reserved");
        }
    } else if ($(this).hasClass("up")) {
        obj = $(".node.up");
        for (var i = 1; i < reservations.length; i++) {
            for (var j = 0; j < reservations[i].Nodes.length; j++) {
                if (!reservations[0].Nodes.includes(reservations[i].Nodes[j])) {
                    getObjFromResIndex(i).addClass("hover");
                    break;
                }
            }
        }
    } else if ($(this).hasClass("down")) {
        obj = $(".node.down");
        for (var i = 1; i < reservations.length; i++) {
            for (var j = 0; j < reservations[i].Nodes.length; j++) {
                if (reservations[0].Nodes.includes(reservations[i].Nodes[j])) {
                    getObjFromResIndex(i).addClass("hover");
                    break;
                }
            }
        }
    }
    obj.removeClass("noshadow");
    obj.addClass("shadow");
});

// undo hover for nodes and reservations
$(".key").mouseleave(function() {
    $(".node").removeClass("noshadow");
    $(".node").removeClass("shadow");
    $(".res").removeClass("hover");
});

// show/hide key with Key button in navbar
$("#keybtn").click(function() {
    $(this).toggleClass("active");
    $("#key").toggle();
});
