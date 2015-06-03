"use strict";

$(document).ready(function () {

////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////
// GENERATE GRAPH STRUCTURE FROM VM INFO ///////////////////////////////////////////////////////////////////////////////////
////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////

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
    var nodeIndexFromValue = [];    // Table correlating a vlan number and its position in graph.nodes
    var unconnectedMachines = [];   // List of machines that aren't on any vlan
    var graph = {
        "nodes": [],
        "links": []
    };

    var vlanFieldIndex = graphFieldNames.indexOf("vlan");
    var vlanNameIndex = graphFieldNames.indexOf("name");

    // generate vlanList
    graphData.forEach(function (currentVM, i, array) {
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
            vlanWeight[vlan] = (vlanWeight[vlan] === undefined) ? 0 : vlanWeight[vlan] + 1;
            if (vlanList.indexOf(vlan) == -1) vlanList.push(vlan);
            if (vlanMachines[vlan] === undefined) {
                vlanMachines[vlan] = [vmName];
            } else {
                vlanMachines[vlan].push(vmName);
            }
        });
    });

    vlanList.forEach(function (vlan, i, array) {
        graph.nodes.push({"name": vlan, "group": 0, "unconnected": false});
        nodeIndexFromValue[vlan] = graph.nodes.length - 1;
        vlanLinks[vlan] = [];
    });

    unconnectedMachines.forEach(function (machine, i, array) {
        graph.nodes.push({"name": machine, "group": 1, "unconnected": true});
    });

    // generate the vlanLinks list
    //TODO: fix me!!
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
                    "value": links.filter(function (x) { return x == currentTarget }).length    // # of times currentTarget occurs in links
                });
            }
        }
    });

////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////
// GENERATE SCALING FOR NODES AND LINKS ////////////////////////////////////////////////////////////////////////////////////
////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////

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

    var radiusScale = d3.scale.pow().exponent(.5)
        .domain([minVlanWeight, maxVlanWeight])
        .range([10, 22]);
    var linkWidthScale = d3.scale.pow().exponent(2)
        .domain([minLinkWeight, maxLinkWeight])
        .range([1, 15]);

////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////
// ACTUALLY DRAW THE CHART AND SET UP HANDLERS /////////////////////////////////////////////////////////////////////////////
////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////

    var chart = d3.select("#chart");
    var chartHeight = chart.node().getBoundingClientRect()["height"];
    var chartWidth = chart.node().getBoundingClientRect()["width"];

    var color = d3.scale.category20();

    var force = d3.layout.force()
        .charge(-600)
        .linkDistance(80)
        .size([chartWidth, chartHeight]);

    var svg = chart.append("svg")
        .attr("width", chartWidth)
        .attr("height", chartHeight)

    var graphContainer = svg.append("g").attr("id", "graph-container");

    force
        .nodes(graph.nodes)
        .links(graph.links)
        .start();

    var link = graphContainer.selectAll(".link")
            .data(graph.links)
        .enter().append("line")
            .attr("class", "link")
            .style("stroke-width", function (d) { return linkWidthScale(d.value); });

    var node = graphContainer.selectAll("#graph-container > g").data(graph.nodes).enter()
            .append("g")
                .attr("data-node", function (d, i) { return i; })
                .on("mouseenter", function (d) {
                    var popup = d3.select('#popup > *[data-node="' + d3.select(this).attr("data-node") + '"]');
                    popup.style("display", "block");
                }).on("mouseleave", function (d) {
                    var popup = d3.select('#popup > *[data-node="' + d3.select(this).attr("data-node") + '"]');
                    popup.style("display", "none");
                });

    var circle = node.append("circle")
            .attr("class", "node")
            .attr("r", function (d) {
                if (!d.unconnected)
                    return radiusScale(vlanWeight[Number(d.name)]);
                else
                    return radiusScale(1);
            })
            .style("fill", function (d) { return color(d.group); });

    node.append("text")
            .attr("class", "vlan-number")
            .attr("y", "5")
            .text(function (d) {
                if (!d.unconnected)
                    return d.name;
                else
                    return " ";
            });

    var popupContainer = svg.append("g")
                    .attr("id", "popup");
    var popup = popupContainer.selectAll("g").data(graph.nodes).enter().append("g")
                    .attr("data-node", function (d, i) { return i; })
    popup.append("rect");
    popup.append("polygon");

    popup.append("text")
            .attr("class", "vlan-machine")
        .selectAll("tspan")
        .data(function (d) {
                if (!d.unconnected)
                    return vlanMachines[Number(d.name)];
                else
                    return [d.name];
        })
        .enter()
            .append("tspan")
            .attr("x", 0)
            .attr("dy", "1.2em")
            .text(function (d) { return d; });
    
    var rectPadding = 5;
    popup.selectAll("rect")
        .data(function (d, i) { return [d3.selectAll(".vlan-machine")[0][i]]; })
            .attr("x", 0)
            .attr("y", 0)
            .attr("width", function (d) { return d.getBBox()["width"] + (2 * rectPadding); })
            .attr("height", function (d) { return d.getBBox()["height"] + (2 * rectPadding); })
            .attr("rx", rectPadding)
            .attr("ry", rectPadding)
            .attr("fill", "#000");

    popup.selectAll("polygon")
        .data(function (d, i) { return [d3.selectAll(".vlan-machine")[0][i]]; })
            .attr("x", 0)
            .attr("y", 0)
            .attr("points", "0,10 10,0 10,20")
            .attr("fill", "#000");

    force.on("tick", function () {
        link.attr("x1", function (d) { return d.source.x; })
            .attr("y1", function (d) { return d.source.y; })
            .attr("x2", function (d) { return d.target.x; })
            .attr("y2", function (d) { return d.target.y; });

        var factor = rezoom();
        node.attr("transform", function (d) { return "translate(" + d.x + "," + d.y + ")"; });
        popup.attr("transform", function (d) {
            var offset = d3.transform(d3.select("#graph-container").attr("style").split(" s")[0].slice(10).split("px").join("")).translate; // Really sorry for this one.
            var centeringOffset = this.getBoundingClientRect()["height"] / 2;
            offset[0] += d3.select('#graph-container > *[data-node="' + d3.select(this).attr("data-node") + '"]').node().getBoundingClientRect()["width"] / 2;
            offset[1] -= centeringOffset;
            d3.select(this).select("* > polygon").attr("transform", "translate(-7, " + (centeringOffset - 10) + ")");
            d3.select(this).select("* > text").attr("transform", "translate(" + rectPadding + ", 0)");
            return "translate(" + ((d.x * factor) + offset[0]) + "," + ((d.y * factor) + offset[1]) + ")";
        });
    });


////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////
////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////
////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////

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
            var node = d3.select(element);
            var popup = d3.select('#popup > *[data-node="' + node.attr("data-node") + '"]');

            var nodeBounds = node.node().getBoundingClientRect();
            if (watch) debugger;
            popup.attr("transform", "translate(" + (nodeBounds["left"] - (nodeBounds["width"] / 2)) + ", " + (nodeBounds["top"] - (nodeBounds["height"] / 2))  + ")");
        });
    }

    // Don't ask.
    function rezoomString (factor, bounds, css) {
        return "translate(" + ((bounds["x"] + (bounds["width"]  / 2)) * (1 - factor)) + (css ? "px" : "") + ", "
                            + ((bounds["y"] + (bounds["height"] / 2)) * (1 - factor)) + (css ? "px" : "") + ") "
             + "scale(" + factor + ", " + factor + ")";
    }

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
});