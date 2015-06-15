"use strict";

// Configurable bits for the graph
var config = {
    types: {                // Types of nodes.
        empty:          0,  // Higher numbers have higher priority. When there are machines of more than one type on a
        normal:         1,  //  vlan, the highest-numbered color in that vlan will be used to color all of that vlan's node.
        router:         2,
        unconnected:    3,
        infected:       4,
        highlighted:    5,
        ethereal:       6 },
    node: {
        size: {                 // Min and max node size
            min:    10,
            max:    30 },
        ethers:         1,      // Number of ethereal nodes that should be generated
        outlineSize:    5 },    // Size of the outline around a node when it is clicked
    zoom: {
        nearLimit:  2.00,       // Don't allow user to zoom in closer than this
        farLimit:   0.02,       // Don't allow user to zoom out farther than this
        rate:       0.05 },     // How quickly we zoom in (higher -> bigger steps)
    force: {
        charge:         -10000, // Repulsion between nodes
        gravity:        0.09,   // Gravity pulling nodes to the center of the graph
        linkStrength:   0.35,   // How strong the links are (lower -> more stretchy)
        linkDistance:   100,    // Normal distance for the links
        friction:       0.1 },  // How quickly all forces slow down (1 -> frictionless, 0 -> no movement)
    selectors: {
        chart:              "#chart",
        sidebarContainer:   "#node-data",
        sidebarHeading:     "#node-name",
        sidebarCount:       "#node-count",
        sidebarSubnodes:    "#subnodes",
        sidebarText: {                          // Default text for a given selector element (used to revert sidebar text to a specific string when no node is selected)
            "sidebarHeading":   "Click a node",
            "sidebarCount":  "" }}
};

// Initialize the color palette
config.palette = (function () {     // IIFEs are pretty cool.
    var colors = [];

    colors[config.types.empty      ] = "#A3A3A3";
    colors[config.types.normal     ] = "#2C3E50";
    colors[config.types.router     ] = "#E7C03C";
    colors[config.types.unconnected] = "#2980B9";
    colors[config.types.infected   ] = "#E74C3C";
    colors[config.types.highlighted] = "#ECF0F1";
    colors[config.types.ethereal   ] = "#000000";

    return colors;
})();


// Set the color palette to be used by Grapher
Grapher.setPalette(null, config.palette);


// Global variable containing all info on the main graph
var grapher = {
    background:     null,       // Background color of the graph container
    instance:       null,       // Instance of the Grapher object (used for all the WebGL graphing magic)
    d3force:        null,       // Instance of the D3 force-directed graph object (used for positioning the nodes)
    selectedNode:   null,       // The currently-selected node (the one you clicked on to select and display its info)
    jsonString:     null,       // The most recently-received JSON string containing all the graph data. The graph is only updated when this changes.
    graph:          {
        "nodes":  [],           // List of nodes in our graph
        "links":  [],           // List of links in our graph
        "ethers": 0             // Number of circles drawn on the graph that should be excluded from the force calculations.
    }                           //  IE just circles that can be drawn on the graph by hijacking the Grapher drawing interface
};


// Global variable containing all info on cursor events
var cursor = {
    event:          null,       // We assign document.onmousemove to update this variable with its event object. This way we always have cursor position.
    node:           null,       // The node that is the target of a mousedown event
    startPoint:     null,       // Used for panning
    movedTo:        null,       // Assigned on graph mousemove events... used for dragging and panning
    hoveringOver:   null        // The node that is currently being hovered over by the mouse
};


////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////
////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////
////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////


// Make it so that we always have the cursor position
document.onmousemove = function (e) {
    cursor.event = e;
}


////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////


// Returns the ID of the node at a given point on the current grapher window
function getNodeIdAt (point, network) {
    var node = -1,
        x    = point.x,
        y    = point.y;

    network.nodes.every(function (n, i) {
        if (n.group == config.types.ethereal) return true;   // Ignore ethereal nodes
        var inX = x <= n.x + n.r && x >= n.x - n.r,
            inY = y <= n.y + n.r && y >= n.y - n.r,
            found = inX && inY;
        if (found) node = i;
        return !found;
    });

    return node;
};


// Helper function for offsets. Used for grapher
function getOffset (e, canvas) {
    if (e.offsetX) return {x: e.offsetX, y: e.offsetY};
    var rect = canvas.getBoundingClientRect();
    var x = e.clientX - rect.left,
        y = e.clientY - rect.top;
    return {x: x, y: y};
};


// Get the node located at the coordinates of the event
function eventNode (e) {
    var point = grapher.instance.getDataPosition(getOffset(e, grapher.instance.canvas));
    return getNodeIdAt(point, grapher.graph);
}


// Update cursor.hoveringOver with the appropriate value for the new cursor position
function checkHover (e) {
    var nodeId = eventNode(e);

    if (nodeId > -1) {
        d3.select(config.selectors.chart).style("cursor", "pointer");

        if (nodeId != grapher.selectedNode) {
            setColor(nodeId, config.types.highlighted);
            cursor.hoveringOver = nodeId;
        }

    // If we WERE hovering over a node, and it's not the currently selected node...
    } else if ((cursor.hoveringOver != null)) {
        d3.select(config.selectors.chart).style("cursor", "default");

        if (cursor.hoveringOver != grapher.selectedNode) {
            setColor(cursor.hoveringOver);
            cursor.hoveringOver = null;
        }
    }
}


////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////


// Set the appropriate color for a VM
function vmColor (vm, hex) {
    var colorNumber;

    if (vm.tags.infected == "true") {
        colorNumber = config.types.infected;
    } else {
        colorNumber = config.types[vm.type];
    }

    if (hex) {
        return config.palette[colorNumber];
    } else {
        return colorNumber;
    }
}


// Calculate the color for a node. Color is determined by getting the highest-priority color of all
//  VMs in the node
function nodeColor (node, hex) {
    for (var i = 0; i < node.machines.length; i++) {
        var vm = node.machines[i];
        vm.color = vmColor(vm);
    }

    var maxPriorityColor = 0;
    for (var i = 0; i < node.machines.length; i++) {
        if (maxPriorityColor < vmColor(node.machines[i]))
            maxPriorityColor = vmColor(node.machines[i]);
    }

    if (hex) {
        return config.palette[maxPriorityColor];
    } else {
        return maxPriorityColor;
    }
}


// Set a node to a color. If color is not specified, the node is reset to its default color.
function setColor (id, color) {
    if (color === undefined) color = nodeColor(grapher.graph.nodes[id]);
    grapher.graph.nodes[id].color = color;
    grapher.instance.update();
}


// Move the first node (which should be ethereal) to be at the given node's position and make it bigger by
//  a certain amount to give the appearance of outlining a node.
function outlineNode (id) {
    var ether = grapher.graph.nodes[0];
    if (ether != undefined) {
        if (id != null) {
            var follow = grapher.graph.nodes[id];
            ether.x = follow.x;
            ether.y = follow.y;
            ether.r = follow.r + config.node.outlineSize;
            ether.color = config.types.ethereal;
        } else {
            ether.r = 0;
            ether.color = grapher.background;
        }
    }
}


////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////


// List all the machines that the selected node directly touches
function listMachines (ul, id) {
    var vlans = grapher.graph.nodes[id].vlans;
    var allMachines = {};

    for (var i = 0; i < vlans.length; i++) {
        var vlan = vlans[i];
        var vlanMachines = grapher.graph.machines.vlans[vlan];

        for (var j = 0; j < vlanMachines.length; j++) {
            var machineId = vlanMachines[j].id;

            if (allMachines[machineId] === undefined) {        // Add a machine to the list if it's not already in there
                allMachines[machineId] = vlanMachines[j];
            }
        }
    }

    for (var machine in allMachines) {
        var li = ul.append("li");
        li.text(allMachines[machine].name);
        li.style("color", vmColor(allMachines[machine], true));
    }
}


// Set sidebar info to be about a specific node
function setSidebarNode (id) {
    var ul = d3.select(config.selectors.sidebarSubnodes);
    var header = d3.select(config.selectors.sidebarHeading);
    var subheader = d3.select(config.selectors.sidebarCount);
    var container = d3.select(config.selectors.sidebarContainer);

    for (var selector in config.selectors.sidebarText)
        d3.select(config.selectors[selector]).text(config.selectors.sidebarText[selector]);

    ul.html("");
    header.style("color", "");

    if (id === null) {
        container.attr("class", "uninitialized");
    } else {
        container.attr("class", "");

        if (grapher.graph.nodes[id].group == config.types.normal) {
            header.text("VLAN " + grapher.graph.nodes[id].vlans[0]);
            subheader.text(grapher.graph.nodes[id].machines.length + " VM" + ((grapher.graph.nodes[id].machines.length > 1) ? "s" : ""));

            listMachines(ul, id);
        } else if (grapher.graph.nodes[id].group == config.types.router) {
            header.text(grapher.graph.nodes[id].machines[0].name);
            subheader.text("Router");
            
            listMachines(ul, id);
        } else if (grapher.graph.nodes[id].group == config.types.empty) {
            header.text("VLAN " + grapher.graph.nodes[id].vlans[0]);
            subheader.text("Empty");

            listMachines(ul, id);
        } else if (grapher.graph.nodes[id].group == config.types.unconnected) {
            header.text(grapher.graph.nodes[id].machines[0].name);
            subheader.text("Unconnected");
        }
        
        subheader.style("color", nodeColor(grapher.graph.nodes[id], true));
    }
}


////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////


// Used to get the new graph info via AJAX.
function getVMInfo () {
    d3.text("/info", function (error, info) {
        if ((info != grapher.jsonString) && (cursor.node == null)) {
            if (error) return console.warn(error);

            grapher.jsonString = info;
            var json = JSON.parse(info);

            var oldGraph = grapher.graph;
            grapher.graph = makeGraph(json, 1);

            for (var i = 0; i < grapher.graph.nodes.length; i++) {
                var node = grapher.graph.nodes[i];

                // Match each new node with its corresponding previous node
                var oldNode = oldGraph.nodes.filter(function (candidateOldNode, j) {
                    if (candidateOldNode.id === node.id) {
                        if (grapher.selectedNode === j)
                            grapher.selectedNode = i;
                            setSidebarNode(i);
                        return true;
                    }

                    return false;
                })[0];

                // Preserve location and other information
                if (oldNode != undefined) {
                    node.x = oldNode.x;
                    node.y = oldNode.y;
                    node.px = oldNode.px;
                    node.py = oldNode.py;
                    node.fixed = oldNode.fixed;
                    node.selected = oldNode.selected;
                }

                node.color = nodeColor(node);
            };

            grapher.d3force
                .nodes(grapher.graph.nodes.slice(config.node.ethers))
                .links(grapher.graph.links)
                .start()
                .alpha(2.5);
            grapher.instance.data(grapher.graph);
        }
    });
}


////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////
////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////
////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////


// Update the min and max fields in the limits object based on the value (current) passed in
function updateLimits (current, limits) {
    if (current > limits.max) limits.max = current;
    if (current < limits.min) limits.min = current;
}


// Add an item to an array if the array exists, or initialize a new array with the item
function pushOrCreate (object, key, value) {
    if (object[key] === undefined)
        object[key] = [value];
    else object[key].push(value);
}


// Generate graph structure from VM info
function makeGraph (response, ethers) {
    var vlans = [];
    var routers = [];
    var unconnected = [];
    var nodeIndexFromVlanNumber = {};

    var network = {
        "nodes": [],
        "links": [],
        "machines": {
            "unconnected": [],
            "vlans": []
        }
    };

    var nodeLimits = {
        max: -Infinity,
        min: Infinity
    };

    // The response is broken down by hosts. Loop through the machines in each host and populate
    //  the lists of machines accordingly.
    for (var host in response) {
        for (var i = 0; i < response[host].length; i++) {         // for vm in host
            var vm = response[host][i];

            vm["host"] = host;
            vm["type"] = null;

            // Router (multiple VLANs)
            if (vm.vlan.length > 1) {
                vm["type"] = "router";
                routers.push(vm);

                for (var j = 0; j < vm.vlan.length; j++)
                    pushOrCreate(network.machines.vlans, vm.vlan[j], vm);

            // Unconnected machine (no VLANs)
            } else if (vm.vlan.length < 1) {
                vm["type"] = "unconnected";
                unconnected.push(vm);
                network.machines.unconnected.push(vm);

            // Normal machine (one VLAN)
            } else {
                vm["type"] = "normal";
                pushOrCreate(vlans, vm.vlan[0], vm);
                pushOrCreate(network.machines.vlans, vm.vlan[0], vm);
            }
        }
    }

    // The first node is drawn under all the rest. This node is for providing a visual cue that a node is selected.
    for (var i = 0; i < ethers; i++) {
        network.nodes.push({
            "vlans":        [],
            "group":        config.types.ethereal,
            "color":        config.types.ethereal,
            "unconnected":  false,
            "machines":     [],
            "id":           "ETHEREAL-" + i,
            "r":            0,
        });
    }

    // VLANs need to be processed first, as the routers depend on them to be there to properly
    //  configure linkages.
    // Add a node for each VLAN
    for (var vlan in vlans) {                           // for vlan in vlans
        var index = network.nodes.push({
            "vlans":        [vlan],
            "group":        config.types.normal,
            "color":        null,
            "unconnected":  false,
            "machines":     vlans[vlan],
            "id":           "VLAN-" + vlan,
            "r":            null,
        }) - 1;

        network.nodes[index].color = nodeColor(network.nodes[index]);
        nodeIndexFromVlanNumber[vlan] = index;

        updateLimits(vlans[vlan].length, nodeLimits);
    }

    // Add a node and properly link each router to its VLANs
    for (var i = 0; i < routers.length; i++) {          // for router in routers
        var router = routers[i];

        var index = network.nodes.push({
            "vlans":        router.vlan,
            "group":        config.types.router,
            "color":        vmColor(router),
            "unconnected":  false,
            "machines":     [router],
            "id":           router.uuid,
            "r":            null
        }) - 1;

        for (var j = 0; j < router.vlan.length; j++) {      // for vlan in router.vlan
            var vlan = router.vlan[j];

            var from = index;
            var to   = nodeIndexFromVlanNumber[vlan];

            if (to === undefined) {
                var to = network.nodes.push({
                    "vlans":        [vlan],
                    "group":        config.types.empty,
                    "color":        null,
                    "unconnected":  false,
                    "machines":     [],
                    "id":           "VLAN-" + vlan,
                    "r":            null,
                }) - 1;

                network.nodes[to].color = nodeColor(network.nodes[to]);
                nodeIndexFromVlanNumber[vlan] = to;

                updateLimits(0, nodeLimits);
            }

            network.links.push({
                "from": from,
                "to": to,
                "source": from - ethers,
                "target": to - ethers,
                "value": 1,
                "id": (from - ethers) + "->" + (to - ethers)
            });
        }

        updateLimits(1, nodeLimits);
    }

    // Add a node for each unconnected VM (no specified VLAN)
    for (var i = 0; i < unconnected.length; i++) {      // for vm in unconnected
        var vm = unconnected[i];

        network.nodes.push({
            "vlans":        [],
            "group":        config.types.unconnected,
            "color":        vmColor(vm),
            "unconnected":  true,
            "machines":     [vm],
            "id":           vm.uuid,
            "r":            null
        });

        updateLimits(1, nodeLimits);
    }

    // Generate scaler for node size
    var nodeScale = d3.scale
        .pow()
        .exponent(.75)
        .domain([
            nodeLimits.min,
            nodeLimits.max
        ]).range([
            config.node.size.min,
            config.node.size.max
        ]);

    // Apply scaler to each node
    for (var i = 0; i < network.nodes.length; i++) {    // for node in network.nodes
        var node = network.nodes[i];
        if (node.group != config.types.ethereal) {
            node.r = nodeScale(node.machines.length);
        }
    }

    return network;
}


////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////
////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////
////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////


$(document).ready(function () {
    // RGB to hex. One-liners ftw? Sorry for the ugliness, there's no pretty way to do this AFAIK
    grapher.background = "#" + window.getComputedStyle(d3.select(config.selectors.chart).node(), null)["background-color"]  // "rgb(255, 255, 255)"
                                .slice(4, -1)                                                                               // "255, 255, 255"
                                .split(", ")                                                                                // ["255", "255", "255"]
                                .map(function (b) {
                                    return Number(b).toString(16) })                                                        // ["FF", "FF", "FF"]
                                .join("");                                                                                  // "FFFFFF"

	grapher.instance = new Grapher()
        .palette(null)
        .data(grapher.graph);

    // Set up event handler for mousedown on graph
    grapher.instance.on('mousedown', function (e) {
        var nodeId = eventNode(e);

        if (nodeId > -1) {  // A node was clicked.
            cursor.node = grapher.graph.nodes[nodeId];
        } else {            // A node wasn't clicked. We should pan.
            cursor.startPoint = getOffset(e, grapher.instance.canvas);
        }
    });

    // Set up event handler for mousemove. It's applied to the document and not the graph because
    //  being able to pan using the cursor outside of the graph area is useful.
    $(document).on('mousemove', function (e) {
        if ((cursor.startPoint != null) || (cursor.node != null))
            cursor.movedTo = grapher.instance.getDataPosition(getOffset(e, grapher.instance.canvas));

        // If we're panning
        if (cursor.startPoint != null) {
            if (cursor.node == null) {   // panning
                var translate = grapher.instance.translate(),
                    offset = getOffset(e, grapher.instance.canvas);

                translate[0] += (offset.x - cursor.startPoint.x);
                translate[1] += (offset.y - cursor.startPoint.y);

                cursor.startPoint = offset;
                grapher.instance.translate(translate);
            }
        } else if (cursor.node != null) {           // dragging
            grapher.d3force.alpha(0.5); // nudge the graph
        } else {
            checkHover(e);
        }
    });

    // Function used by both the mouseup and mouseleave handlers
    function endMouseActions (e) {
        // If we're panning and the mouse leaves the chart area, we don't care.
        if ((e.type == "mouseleave") && (cursor.startPoint != null)) return;

        if (cursor.movedTo == null) {   // If the cursor hasn't moved...
            var nodeId = eventNode(e);
            
            if (nodeId > -1) {          // And we clicked on a node...

                // If the selected node isn't null and the selected node isn't the newly clicked node...
                if ((grapher.selectedNode != null) && (grapher.selectedNode != nodeId)) {
                    setColor(grapher.selectedNode);
                }
                
                outlineNode(nodeId);

                setColor(nodeId);
                setSidebarNode(nodeId);

                grapher.selectedNode = nodeId;

            // ... But if we didn't click on a node and the mouseup event called us, clear the selection.
            } else if ((grapher.selectedNode != null) && (e.type == "mouseup")) {
                outlineNode(null);

                setColor(grapher.selectedNode);
                setSidebarNode(null);

                grapher.selectedNode = null;
            }
        }

        // Clear mouse events only relevant when a button is down
        cursor.node = null;
        cursor.startPoint = null;
        cursor.movedTo = null;
    }

    // Always call endMouseActions for mouseup
    grapher.instance.on('mouseup', endMouseActions);

    // If we're doing anything but dragging, clean up mouse actions.
    // This allows us to pan, but not drag nodes outside of the graph area.
    grapher.instance.on('mouseleave', endMouseActions);

    // Zoom in or out on a point when the scroll wheel is moved
    grapher.instance.on('wheel', function (e) {
        // Make sure we are OK to zoom in or out according to the limits
        if (((grapher.instance.renderer.scale > config.zoom.farLimit ) && (e.deltaY > 0)) ||      // Can't zoom out too far...
            ((grapher.instance.renderer.scale < config.zoom.nearLimit) && (e.deltaY < 0))         // ... or in too far
        ) {
            grapher.instance.zoom(1 - (e.deltaY / (1/config.zoom.rate)), getOffset(e, grapher.instance.canvas));
            grapher.instance.render();
        }
    });

    // Force graph with a set of parameters that scales decently well with thousands of nodes
    grapher.d3force = d3.layout.force()
        .gravity(      config.force.gravity      )
        .linkStrength( config.force.linkStrength )
        .linkDistance( config.force.linkDistance )
        .friction(     config.force.friction     )
        .charge(       config.force.charge       )
        .on('tick', function () {
            if ((cursor.node != null) && (cursor.movedTo != null)) {
                cursor.node.x = cursor.movedTo.x;
                cursor.node.y = cursor.movedTo.y;
            }

            if (cursor.event && (cursor.node == null)) {
                checkHover({
                    target:  grapher.instance.canvas,
                    clientX: cursor.event.clientX,
                    clientY: cursor.event.clientY
                });
            }

            if (grapher.selectedNode != null) {
                outlineNode(grapher.selectedNode);
            }

            grapher.instance.update();
        });

    // Wrap things up and get started!
    $(window).resize();

    setSidebarNode(null);
    getVMInfo();
    outlineNode(null);

    d3.select(config.selectors.chart).node().appendChild(grapher.instance.canvas);
    grapher.instance.play();

    setInterval(getVMInfo, 500);
});


// Resize the graph when the window is resized
$(window).resize(function () {
    if (grapher.instance != null) {
        var bounds = d3.select(config.selectors.chart).node().getBoundingClientRect();
        grapher.d3force.size([bounds.width, bounds.height]).resume();
    }
});