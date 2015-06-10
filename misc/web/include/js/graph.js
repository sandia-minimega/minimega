"use strict";

// We have a few global-ish vars...
var force,
    grapher,
    cursor,
    highlighted = null,
    oldInfo = "",
    graph = {
        "nodes": [],
        "links": []
    };

var dragging = null,
    offset = null,
    startPoint = null,
    inspecting = null,
    bounds = d3.select("#chart").node().getBoundingClientRect();

////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////
////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////
////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////

function getNodeIdAt (point, network) {
    var node = -1,
        x    = point.x,
        y    = point.y;

    network.nodes.every(function (n, i) {
      var inX = x <= n.x + n.r && x >= n.x - n.r,
          inY = y <= n.y + n.r && y >= n.y - n.r,
          found = inX && inY;
      if (found) node = i;
      return !found;
    });

    return node;
};


// Helper function for offsets.
function getOffset (e) {
    if (e.offsetX) return {x: e.offsetX, y: e.offsetY};
    var rect = e.target.getBoundingClientRect();
    var x = e.clientX - rect.left,
        y = e.clientY - rect.top;
    return {x: x, y: y};
};


//
function selectNode (id) {
    d3.select("#node-heading > h1").text(graph.nodes[id].name);
    d3.select("#node-count").text(graph.nodes[id].count + " VM" + ((graph.nodes[id].count > 1) ? "s" : ""));
}


//
function update () {
    d3.text("/info", function (error, info) {
        if ((info != oldInfo) && (!dragging)) {
            if (error) return console.warn(error);

            oldInfo = info;
            var json = JSON.parse(info);

            var oldGraph = graph;
            graph = makeGraph(json.Header, json.Tabular);

            graph.nodes.forEach(function (node, i, array) {
                var oldNode = oldGraph.nodes.filter(function (candidateOldNode) {
                    return candidateOldNode.id === node.id;
                })[0];

                if (oldNode != undefined) {
                    node.x = oldNode.x;
                    node.y = oldNode.y;
                    node.px = oldNode.px;
                    node.py = oldNode.py;
                    node.fixed = oldNode.fixed;
                }
            });

            force
                .nodes(graph.nodes)
                .links(graph.links)
                .start()
                .alpha(2.5);
            grapher.data(graph);
        }
    });
}


function checkHover (e) {
    var point = grapher.getDataPosition(getOffset(e));
    var nodeId = getNodeIdAt(point, graph);

    if (nodeId > -1) {
        d3.select("#chart").style("cursor", "pointer");
        graph.nodes[nodeId].color = 2;
        highlighted = nodeId;
        grapher.update();
    } else {
        d3.select("#chart").style("cursor", "default");
        if (highlighted != null) {
            graph.nodes[highlighted].color = graph.nodes[highlighted].originalColor;
            highlighted = null;
            grapher.update();
        }
    }
}

////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////
////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////
////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////

// Generate graph structure from VM info
function makeGraph (fieldNames, data) {
    var vlanList = [];      // List of all existing vlans
                            //     EXAMPLE: [0, 1, 2, 5]
    var multipleVlans = []; // List of all instances where vlans are connected together (VM with multiple vlans)
                            //     EXAMPLE: [[1, 2], [1, 2], [2, 5], [0, 1, 2]]
    var vlanWeight = [];    // List of weights for all vlans (how often they occur)
                            //     EXAMPLE: vlanWeight[0] = 1
                            //     EXAMPLE: vlanWeight[1] = 3
                            //     EXAMPLE: vlanWeight[2] = 4
                            //     EXAMPLE: vlanWeight[5] = 1
    var vlanLinks = [];     // List, indexed by vlan, that contains all connections from vlan to vlan. Connections are considered unidirectional.
                            // The smaller vlan number is used as the source, and the larger as the target. Vlans may appear multiple times. This yields a higher weight.
                            //     EXAMPLE: vlanLinks[0] = [1, 2]
                            //              vlanLinks[1] = [2, 2, 2]
                            //              vlanLinks[2] = [5]
                            //              vlanLinks[5] = []
    var vlanMachines = [];  // Machines on each vlan
    var nodeIndexFromValue = [];    // Table correlating a vlan number and its position in returnGraph.nodes
    var unconnectedMachines = [];   // List of machines that aren't on any vlan
    var returnGraph = {
        "nodes": [],
        "links": []
    };

    var vlanFieldIndex = fieldNames.indexOf("vlan");
    var vlanNameIndex = fieldNames.indexOf("name");

    // generate vlanList
    data.forEach(function (currentVM, i, array) {
        var vmName = currentVM[vlanNameIndex];
        var vlanString = currentVM[vlanFieldIndex];
        if (vlanString == "[]") {
            unconnectedMachines.push(vmName);
            return;
        }

        // If we haven't already returned, the VM has at least one vlan assigned
        var vlans = vlanString.slice(1, -1).split(" ").map(Number).sort();
        if (vlans.length > 1) multipleVlans.push(vlans);
        vlans.forEach(function (vlan, i, array) {
            vlanWeight[vlan] = (vlanWeight[vlan] === undefined) ? 1 : vlanWeight[vlan] + 1;
            if (vlanList.indexOf(vlan) == -1) vlanList.push(vlan);
            if (vlanMachines[vlan] === undefined) {
                vlanMachines[vlan] = [vmName];
            } else {
                vlanMachines[vlan].push(vmName);
            }
        });
    });

    vlanList.forEach(function (vlan, i, array) {
        returnGraph.nodes.push({
            "name": vlan,
            "group": 0,
            "color": 0,
            "originalColor": 0,
            "unconnected": false,
            "count": vlanWeight[vlan],
            "machines": vlanMachines[vlan],
            "id": "VLAN-" + vlan,
            "r": 20
        });
        nodeIndexFromValue[vlan] = returnGraph.nodes.length - 1;
        vlanLinks[vlan] = [];
    });

    unconnectedMachines.forEach(function (machine, i, array) {
        returnGraph.nodes.push({
            "name": "Unconnected VM",
            "group": 1,
            "color": 1,
            "originalColor": 1,
            "unconnected": true,
            "count": 1,
            "machines": [machine],
            "id": "MACHINE-" + machine,
            "r": 20
        });
    });

    // generate the vlanLinks list
    multipleVlans.forEach(function (vlansInMulti, i, array) {
        for (var j = 0; j < vlansInMulti.length; j++) {
            var sourceNode = vlanLinks[vlansInMulti[j]];
            for (var k = j + 1; k < vlansInMulti.length; k++) {
                var linkedNode = vlansInMulti[k];
                sourceNode.push(linkedNode);
            }
        }
    });

    // generate links how d3 wants them
    vlanLinks.forEach(function (links, i, array) {
        var usedLinks = [];
        for (var j = 0; j < links.length; j++) {
            var currentTarget = links[j];
            if (usedLinks.indexOf(currentTarget) == -1) {
                usedLinks.push(currentTarget);
                returnGraph.links.push({
                    "source": nodeIndexFromValue[i],
                    "target": nodeIndexFromValue[currentTarget],
                    "from": nodeIndexFromValue[i],
                    "to": nodeIndexFromValue[currentTarget],
                    "value": links.filter(function (x) { return x == currentTarget }).length,    // # of times currentTarget occurs in links
                    "id": String(returnGraph.nodes[nodeIndexFromValue[i]].id) + "-" + String(returnGraph.nodes[nodeIndexFromValue[currentTarget]].id)
                });
            }
        }
    });

    return returnGraph;
}

document.onmousemove = function (e) {
    cursor = e;
}

////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////
////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////
////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////

$(document).ready(function () {

    var palettes = [
        {   name: "Flat UI",
            colors: ["#2C3E50", "#3498DB", "#ECF0F1", "#E74C3C", "#2980B9"]
        },{ name: "Firenze",
            colors: ["#468966", "#FFB03B", "#FFF0A5", "#B64926", "#8E2800"]
        }];
    palettes.forEach(function (palette, i, arr) {
        Grapher.setPalette(palette.name, palette.colors);
    });

////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////
////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////
////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////

    update();
    
	grapher = new Grapher()
        .palette('Flat UI')
        .color('#ffffff')
        .data(graph);

    // Force graph with a set of parameters that scales decently well with thousands of nodes
    force = d3.layout.force()
		.nodes(graph.nodes)
		.links(graph.links)
		.size([bounds.width, bounds.height])
		.on('tick', function () {
            if (dragging && offset) {
                dragging.node.x = offset.x;
                dragging.node.y = offset.y;
            }

            grapher.update();

            if (cursor) checkHover({
                    target:  d3.select("#chart > canvas").node(),
                    clientX: cursor.clientX,
                    clientY: cursor.clientY });
        })
		.charge(-10000)
		.gravity(0.09)
		.linkStrength(0.35)
		.linkDistance(100)
		.friction(0.1)
		.start()
        .alpha(2.5);

////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////
////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////
////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////

    grapher.on('mousedown', function (e) {
        var point = grapher.getDataPosition(getOffset(e));
        var nodeId = getNodeIdAt(point, graph);

        if (nodeId > -1) {  // A node was clicked.
            inspecting = true;
            dragging = {node: graph.nodes[nodeId], id: nodeId};
        } else {            // A node wasn't clicked. We should pan.
            dragging = offset = null;
            startPoint = getOffset(e);
        }
    });

    grapher.on('mousemove', function (e) {
        if ((dragging == null) && (startPoint != null)) {   // panning
            var translate = grapher.translate(),
                thisOffset = getOffset(e);

            translate[0] += (thisOffset.x - startPoint.x);
            translate[1] += (thisOffset.y - startPoint.y);

            startPoint = thisOffset;
            grapher.translate(translate);
        } else if (dragging != null) {                      // dragging
            inspecting = false;
            var point = grapher.getDataPosition(getOffset(e));

            if (dragging) {
                offset = point;
                force.alpha(0.5); // nudge the graph
            }
        } else {                                            // hovering
            checkHover(e);
        }
    });

    grapher.on('mouseup', function (e) {
        if (inspecting) {
            var point = grapher.getDataPosition(getOffset(e));
            var nodeId = getNodeIdAt(point, graph);

            inspecting = false;
            dragging = offset = null;
            d3.select("#node-data").attr("class", "");
            if (nodeId > -1) selectNode(nodeId);
        } else if ((dragging == null) && (startPoint != null)) {   // panning
            startPoint = null;
        } else if (dragging != null) {                      // dragging
            dragging = offset = null;
        }
    });

    grapher.on('wheel', function (e) {
        if (
            ((grapher.renderer.scale > 0.02) && (e.deltaY > 0)) ||      // Can't zoom out too far...
            ((grapher.renderer.scale < 2   ) && (e.deltaY < 0))         // ... or in too far
        ) {
            grapher.zoom(1 - (e.deltaY / 50), getOffset(e));
            grapher.render();
        }
    });

////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////
////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////
////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////

    d3.select("#chart").node().appendChild(grapher.canvas);
    grapher.play();

////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////
////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////
////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////

    setInterval(update, 250);
});