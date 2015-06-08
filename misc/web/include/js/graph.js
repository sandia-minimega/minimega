"use strict";

// Keep our stuff out of the global scope
// (function () {

// Configurable settings
var config = {
    "rectPadding": 5,
    "colors": [
        "#2C3E50",
        "#E74C3C",
        "#ECF0F1",
        "#3498DB",
        "#2980B9"
    ],
    "scaling": {
        "node": {
            "min": 10,
            "max": 22
        },
        "link": {
            "min": 1,
            "max": 15
        }
    }
};

// We have a few global-ish vars...
var graph, force, graphContainer, popupContainer, node, popup, link, dragging, vlanWeight;
dragging = false;

// Generate graph structure from VM info
function makeGraph (fieldNames, data) {
    var vlanList = [];      // List of all existing vlans
                            //     EXAMPLE: [0, 1, 2, 5]
    var multipleVlans = []; // List of all instances where vlans are connected together (VM with multiple vlans)
                            //     EXAMPLE: [[1, 2], [1, 2], [2, 5], [0, 1, 2]]
    vlanWeight = [];    // List of weights for all vlans (how often they occur)
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
    var nodeIndexFromValue = [];    // Table correlating a vlan number and its position in graph.nodes
    var unconnectedMachines = [];   // List of machines that aren't on any vlan
    graph = {
        "nodes": [],
        "links": [],
        "nodeScale": null,
        "linkScale": null,

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
        graph.nodes.push({
            "name": vlan,
            "group": 0,
            "unconnected": false,
            "count": vlanWeight[vlan],
            "machines": vlanMachines[vlan],
            "id": "VLAN-" + vlan
        });
        nodeIndexFromValue[vlan] = graph.nodes.length - 1;
        vlanLinks[vlan] = [];
    });

    unconnectedMachines.forEach(function (machine, i, array) {
        graph.nodes.push({
            "name": "~",
            "group": 3,
            "unconnected": true,
            "count": 1,
            "machines": [machine],
            "id": "MACHINE-" + machine
        });
        vlanWeight.push(1);
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
                graph.links.push({
                    "source": nodeIndexFromValue[i],
                    "target": nodeIndexFromValue[currentTarget],
                    "value": links.filter(function (x) { return x == currentTarget }).length,    // # of times currentTarget occurs in links
                    "id": String(graph.nodes[nodeIndexFromValue[i]].id) + "-" + String(graph.nodes[nodeIndexFromValue[currentTarget]].id)
                });
            }
        }
    });
}

function makeScales (graph) {
    var vlanWeight = [];
    graph.nodes.forEach(function (node, i, array) {
        vlanWeight.push(node.count);
    });

    var vlanWeightArguments = [];
    for (var i = 0; i < vlanWeight.length; i++) {
        var current = vlanWeight[i];
        if (!isNaN(current)) vlanWeightArguments.push(current);
    }

    var minVlanWeight = Math.min.apply(null, vlanWeightArguments);
    var maxVlanWeight = Math.max.apply(null, vlanWeightArguments);

    var linkWeightArguments = [];
    for (var i = 0; i < graph.links.length; i++) {
        linkWeightArguments.push(graph.links[i].value);
    }

    var minLinkWeight = Math.min.apply(null, linkWeightArguments);
    var maxLinkWeight = Math.max.apply(null, linkWeightArguments);

    graph.nodeScale = d3.scale.pow().exponent(.5)
        .domain([minVlanWeight, maxVlanWeight])
        .range([config.scaling.node.min, config.scaling.node.max]);
    graph.linkScale = d3.scale.pow().exponent(2)
        .domain([minLinkWeight, maxLinkWeight])
        .range([config.scaling.link.min, config.scaling.link.max]);
}

function updateGraph () {
    node = graphContainer.selectAll("#graph-container > g").data(graph.nodes, function (d) { return d.id; });
    var nodeEntered = node.enter()
            .append("g")
                .attr("data-node", function (d) { return d.id; })
                .on("mouseenter", function (d) {
                    var currentPopup = d3.select('#popup > *[data-node="' + d3.select(this).attr("data-node") + '"]');
                    currentPopup.style("display", "block");
                }).on("mouseleave", function (d) {
                    if (!dragging) {
                        var currentPopup = d3.select('#popup > *[data-node="' + d3.select(this).attr("data-node") + '"]');
                        currentPopup.style("display", "none");
                    }
                })
    var nodeDrag = d3.behavior.drag()
                .on("dragstart", function () {
                    dragging = true;
                    force.stop();
                    d3.selectAll("#graph-container").attr("class", "no-transition");
                })
                .on("drag", function (d) {
                    d.px += d3.event.dx;
                    d.py += d3.event.dy;
                    d.x += d3.event.dx;
                    d.y += d3.event.dy; 
                    tick();
                })
                .on("dragend", function () {
                    dragging = false;
                    d3.selectAll("#graph-container").attr("class", "");
                    force.resume();
                    asyncUpdate();
                    var currentPopup = d3.select('#popup > *[data-node="' + d3.select(this).attr("data-node") + '"]');
                    currentPopup.style("display", "none");
                });
    nodeEntered.call(nodeDrag);
    nodeEntered.append("circle")
            .attr("class", "node")
            .attr("r", function (d) { return graph.nodeScale(d.count); })
            .style("fill", function (d) { return config.colors[d.group]; });
    nodeEntered.append("text")
            .attr("class", "vlan-number")
            .attr("y", "5")
            .text(function (d) { return d.name; });
    node.exit().remove();

    // Update the links

    link = graphContainer.selectAll(".link").data(graph.links, function (d) { return d.id; });
    link.enter().insert("line", "#graph-container > g")
        .attr("class", "link")
        .style("stroke-width", function (d) { return graph.linkScale(d.value); });
    link.exit().remove();

    // Update the popups

    popup = popupContainer.selectAll("g").data(graph.nodes, function (d) { return d.id; });
    var popupEntered = popup.enter()
            .append("g")
                .attr("data-node", function (d) { return d.id; })
    var textBoxes = popupEntered.append("text");
        textBoxes
            .attr("class", "vlan-machine")
        .selectAll("tspan")
        .data(function (d) { return d.machines; }, function (d) { return d; }).enter()
            .append("tspan")
            .attr("x", 0)
            .attr("dy", "1.2em")
            .text(function (d) { return d; });
    popupEntered.insert("rect", "text")
        .attr("x", 0)
        .attr("y", 0)
        .attr("width", function (d) {
            try {
                return d3.select('*[data-node="' + d.id + '"] .vlan-machine').node().getBBox()["width"] + (2 * config.rectPadding);
            } catch (error) {
                return 0;
            }
        })
        .attr("height", function (d) {
            try {
                return d3.select('*[data-node="' + d.id + '"] .vlan-machine').node().getBBox()["height"] + (2 * config.rectPadding);
            } catch (error) {
                return 0;
            }
        })
        .attr("rx", config.rectPadding)
        .attr("ry", config.rectPadding)
        .attr("fill", "#000");
    popupEntered.insert("polygon", "text")
        .attr("x", 0)
        .attr("y", 0)
        .attr("points", "0,10 10,0 10,20")
        .attr("fill", "#000");
    popup.exit().remove();

    return {
        "node": node,
        "popup": popup,
        "link": link
    };
}


// Zoom in so that graph fills most of the area
function rezoom () {
    var chart = d3.select("#chart");
    var desiredHeight = chart.node().getBoundingClientRect()["height"] - 50;
    var desiredWidth = chart.node().getBoundingClientRect()["width"] - 50;

    // Rezoom all of the chart.
    var bounds = graphContainer.node().getBBox();
    var heightRatio = desiredHeight / bounds["height"] * 0.75;
    var widthRatio = desiredWidth / bounds["width"] * 0.75;

    var factor = Math.min(heightRatio, widthRatio);
    graphContainer.attr("style", "transform:" + rezoomString(factor, bounds, true));

    return factor;
}


function repositionPopup (watch) {
    d3.selectAll("#graph-container > g")[0].forEach(function (element, i, array) {
        var currentNode = d3.select(element);
        var currentPopup = d3.select('#popup > *[data-node="' + currentNode.attr("data-node") + '"]');

        var nodeBounds = currentNode.node().getBoundingClientRect();
        currentPopup.attr("transform", "translate(" + (nodeBounds["left"] - (nodeBounds["width"] / 2)) + ", " + (nodeBounds["top"] - (nodeBounds["height"] / 2))  + ")");
    });
}


// Don't ask.
function rezoomString (factor, bounds, css) {
    return "translate(" + ((bounds["x"] + (bounds["width"]  / 2)) * (1 - factor)) + (css ? "px" : "") + ", "
                        + ((bounds["y"] + (bounds["height"] / 2)) * (1 - factor)) + (css ? "px" : "") + ") "
         + "scale(" + factor + ", " + factor + ")";
}


$(document).ready(function () {

    makeGraph(graphFieldNames, graphData);
    makeScales(graph);



    window.tick = function () {
        link.attr("x1", function (d) { return d.source.x; })
            .attr("y1", function (d) { return d.source.y; })
            .attr("x2", function (d) { return d.target.x; })
            .attr("y2", function (d) { return d.target.y; });

        var factor = rezoom(graphContainer);
        node.attr("transform", function (d) { return "translate(" + d.x + "," + d.y + ")"; });
        popup.attr("transform", function (d) {
            var offset = d3.transform(d3.select("#graph-container").attr("style").split(" s")[0].slice(10).split("px").join("")).translate; // Really sorry for this one.
            var centeringOffset = this.getBoundingClientRect()["height"] / 2;
            offset[0] += d3.select('#graph-container > *[data-node="' + d3.select(this).attr("data-node") + '"]').node().getBoundingClientRect()["width"] / 2;
            offset[1] -= centeringOffset;
            d3.select(this).select("* > polygon").attr("transform", "translate(-7, " + (centeringOffset - 10) + ")");
            d3.select(this).select("* > text").attr("transform", "translate(" + config.rectPadding + ", 0)");
            return "translate(" + ((d.x * factor) + offset[0]) + "," + ((d.y * factor) + offset[1]) + ")";
        });
    };

////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////
// ACTUALLY DRAW THE CHART AND SET UP HANDLERS /////////////////////////////////////////////////////////////////////////////
////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////

    var chart = d3.select("#chart");
    var chartHeight = chart.node().getBoundingClientRect()["height"];
    var chartWidth = chart.node().getBoundingClientRect()["width"];

    var svg = chart.append("svg")
        .attr("width", chartWidth)
        .attr("height", chartHeight)

    graphContainer = svg.append("g").attr("id", "graph-container");

    force = d3.layout.force()
        .charge(-400)
        .chargeDistance(400)
        .linkDistance(100)
        .size([chartWidth, chartHeight])
        .nodes(graph.nodes)
        .links(graph.links);

    popupContainer = svg.append("g")
                    .attr("id", "popup");

    var results = updateGraph();
    node = results.node;
    popup = results.popup;
    link = results.link;
    
    // Firefox has issues with the css transition used to smooth transforms, so we disable it
    if (document.getBoxObjectFor != null || window.mozInnerScreenX != null) {
        d3.selectAll("#graph-container, #popup > g").attr("class", "no-transition");
    }

    force.start();

    force.on("tick", tick);


////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////
////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////
////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////

    $(window).resize((function (force) {
        return function () {
            var chart = d3.select("#chart");
            var chartHeight = chart.node().getBoundingClientRect()["height"];
            var chartWidth = chart.node().getBoundingClientRect()["width"];

            var svg = d3.select('#chart > svg')
                .attr("width", chartWidth)
                .attr("height", chartHeight);

            force.size([chartWidth, chartHeight]).resume();
        };
    })(force));

    window.asyncUpdate = function () {
        d3.json("/info", function (error, json) {
            if (error) return console.warn(error);
            if (dragging) return null;

            var oldGraph = graph;
            makeGraph(json.Header, json.Tabular);
            makeScales(graph);

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
                .links(graph.links);

            var results = updateGraph();

            node = results.node;
            popup = results.popup;
            link = results.link;

            force.start();
        });
    }

    setInterval(asyncUpdate, 2500)
});

// })();