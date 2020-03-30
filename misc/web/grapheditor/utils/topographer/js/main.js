//------------------------
//       GENERAL
//------------------------
var viewerTopo = {}, viewerSchema = {};

//function loadJson() {
//  var xhr = new XMLHttpRequest();
//  xhr.onreadystatechange = function(){
//    if (xhr.status == 200 && xhr.readyState == 4) {
//      viewerSchema = JSON.parse(xhr.responseText);
//    }
//  };
//  xhr.open("GET","json/schema.json",false);
//  xhr.send();
//
//  var xhr = new XMLHttpRequest();
//  xhr.onreadystatechange = function(){
//    if (xhr.status == 200 && xhr.readyState == 4) {
//      viewerTopo = JSON.parse(xhr.responseText);
//    }
//  };
//  xhr.open("GET","json/topo.json",false);
//  xhr.send();
//}

function initViewer() {
  startJson();
  startMap();
}

function viewerRefresh() {
  updateJson();
  updateMapData(viewerTopo);
}

// data = [{}, {}, ...]
function addNode(data) {
  data.forEach(function(obj) {
    viewerTopo.push(obj);
    updateJson();
    addMapNode(obj);
  });
}

// name = "foobar"
function removeNode(name) {
  var idx = viewerTopo.findIndex(x => x.general.hostname === name);
  if (idx !== -1) {
    viewerTopo.splice(idx, 1);
  }
  updateJson();
  removeMapNode(name);
}

// origName = "foobar", data = {}
function updateNode(origName, data) {
  if (data.type === "Switch") {
    var idx = viewerTopo.vlans.findIndex(x => x.name === origName);
    if (idx !== -1) {
      delete data.type;
      Object.assign(viewerTopo.vlans[idx], data);
    }
    updateNodeVlans(origName, data.name);
  } else {
    var idx = viewerTopo.nodes.findIndex(x => x.general.hostname === origName);
    if (idx !== -1) {
      Object.assign(viewerTopo.nodes[idx], data);
    }
  }
  updateJson();
}

function updateNodeVlans(oldName, newName) {
  viewerTopo.nodes.forEach(function(node) {
    if ('network' in node && 'interfaces' in node.network) {
      node.network.interfaces.forEach(function(iface) {
        if (iface.vlan === oldName) {
          iface.vlan = newName;
        }
      });
    }
  });
}

//------------------------
//         MAP VIEW
//------------------------
var nodes = null, edges = null, edges_no_mgmt = null, network = null;
var COUNT = 1;
var DIR = 'img/nodes/';
var EDGE_LENGTH_MAIN = 150;
var EDGE_LENGTH_SUB = 50;

function startMap() {
  nodes = new vis.DataSet();
  edges = new vis.DataSet();
  edges_no_mgmt = new vis.DataSet();

  // add map data
  updateMapData(viewerTopo);

  // create a network
  var container = document.getElementById('mynetwork');
  var data = {
    nodes: nodes,
    edges: edges_no_mgmt // initialize without mgmt net
  };
  var options = {
    interaction: {
      hover:true
    },
    layout: {
      improvedLayout: false
    },
    physics: {
      solver: 'forceAtlas2Based'
    },
    edges: {
      hoverWidth: function (width) {return width*5;},
      selectionWidth: function (width) {return width*7;}
    }
//    },
//    manipulation: {
//      addNode: function (data, callback) {
//        // filling in the popup DOM elements
//        document.getElementById('operation').innerHTML = "Add Node";
//        document.getElementById('node-type').value = data.json.type;
//        document.getElementById('node-hostname').value = data.json.general.hostname;
//        document.getElementById('node-image').value = data.json.hardware.drives[0].image;
//        document.getElementById('saveButton').onclick = saveData.bind(this, data, callback);
//        document.getElementById('cancelButton').onclick = clearPopUp.bind();
//        document.getElementById('network-popUp').style.display = 'block'; },
//      editNode: function (data, callback) {
//        // filling in the popup DOM elements
//        var operation = "Edit";
//        var rows = [];
//        if (data.json.type === "Switch") {
//          operation = "Edit VLAN";
//          rows = [
//            [{type: "text", value: "name"}, {type: "input", id: "vlan-name", value: data.json.name}],
//            [{type: "text", value: "id"}, {type: "input", id: "vlan-id", value: data.json.id}]
//          ];
//        } else {
//          operation = "Edit Node";
//          rows = [
//            [{type: "text", value: "type"}, {type: "select", id: "node-type", value: data.json.type, options: ['VirtualMachine', 'Router', 'Firewall', 'Printer', 'Server', 'SCEPTRE']}],
//            [{type: "text", value: "hostname"}, {type: "input", id: "node-hostname", value: data.json.general.hostname}],
//            [{type: "text", value: "image"}, {type: "input", id: "node-image", value: data.json.hardware.drives[0].image}]
//          ];
//        }
//        fillPopup(rows);
//        document.getElementById('operation').innerHTML = operation;
//        document.getElementById('saveButton').onclick = saveData.bind(this, data, callback);
//        document.getElementById('cancelButton').onclick = cancelEdit.bind(this, callback);
//        document.getElementById('network-popUp').style.display = 'block';
//      },
//      addEdge: function (data, callback) {
//        if (data.from == data.to) {
//          var r = confirm("Do you want to connect the node to itself?");
//          if (r == true) {
//            callback(data);
//          }
//        }
//        else {
//          callback(data);
//        }
//      }
//    }
  };
  network = new vis.Network(container, data, options);
  network.on("selectNode", function (params) {
    var node = nodes.get(params.nodes[0]);
    jsonDisplay.outputDivID = "nodedetails" ;
    jsonDisplay.outputPretty(JSON.stringify(node.json));
    document.getElementById('nodedetails').style.display = 'inline-block';
    document.querySelectorAll('pre code').forEach((block) => {
      hljs.highlightBlock(block);
    });
  });
  network.on("deselectNode", function (params) {
    document.getElementById('nodedetails').style.display = 'none';
  });
  network.on("stabilizationProgress", function(params) {
    document.getElementById('loadingBar').style.display = 'block';
    document.getElementById('loadingBar').style.opacity = 100;
    var maxWidth = 496;
    var minWidth = 20;
    var widthFactor = params.iterations/params.total;
    var width = Math.max(minWidth,maxWidth * widthFactor);
    document.getElementById('bar').style.width = width + 'px';
    document.getElementById('text').innerHTML = Math.round(widthFactor*100) + '%';
  });
  network.on("stabilizationIterationsDone", function() {
    document.getElementById('text').innerHTML = '100%';
    document.getElementById('bar').style.width = '496px';
    document.getElementById('loadingBar').style.opacity = 0;
    // really clean the dom element
    setTimeout(function () {document.getElementById('loadingBar').style.display = 'none';}, 500);
  });
}

function fillPopup(rows) {
  var table = document.getElementById('popUp-table');
  rows.forEach(function(row) {
    var tr = table.insertRow(-1);
    row.forEach(function(cell) {
      var td = tr.insertCell(-1);
      if (cell.type === "text") {
        td.innerHTML = cell.value;
      } else if (cell.type === "input") {
        var input = document.createElement('input');
        input.id = cell.id;
        input.value = cell.value;
        input.className = "tdinput";
        td.appendChild(input);
      } else if (cell.type === "select") {
        var select = document.createElement('select');
        select.id = cell.id;
        cell.options.forEach(function(opt) {
          if (opt === cell.value) {
            select.options.add(new Option(opt, opt, false, true));
          } else {
            select.options.add(new Option(opt, opt));
          }
        });
        td.appendChild(select);
      }
    });
  });
}

function getId(name) {
  var id_ = -1;
  var items = nodes.get({
    filter: function (item) {
      return item.label === name;
    }
  });
  if (items.length != 0) {
    id_ = items[0].id;
  }
  return id_;
}

function addMapNode(obj) {
  // add vm nodes
  var edgeSrc = COUNT;
  nodes.add({id: COUNT, label: obj.general.hostname, font: {color: 'whitesmoke'}, image: DIR + obj.type + '.png', shape: 'image', json: obj});
  COUNT++;

  // add network nodes
  if ('network' in obj && 'interfaces' in obj.network) {
    var addrs = '';
    obj.network.interfaces.forEach(function(iface) {
      var addr = iface.address == null ? 'auto' : iface.address;
      addrs += iface.name + ': ' + addr + '/' + iface.mask + '<br>';
      var items = nodes.get({
        filter: function (item) {
          return item.label === iface.vlan;
        }
      });
      if (items.length == 0) {
        nodes.add({id: COUNT, label: iface.vlan, font: {color: 'white'}, image: DIR + 'Switch.png', shape: 'image', json: {type: 'Switch', name: iface.vlan, id: 'auto'}, title: 'vlan: auto'});
        var edge = {from: edgeSrc, to: COUNT, length: EDGE_LENGTH_MAIN};
        if (iface.vlan != 'MGMT') {
          edges_no_mgmt.add(edge);
        } else {
          edge['color'] = 'red';
        }
        edges.add(edge);
        COUNT++;
      } else {
        var edge = {from: edgeSrc, to: items[0].id, length: EDGE_LENGTH_MAIN};
        if (iface.vlan != 'MGMT') {
          edges_no_mgmt.add(edge);
        } else {
          edge['color'] = 'red';
        }
        edges.add(edge);
      }
    });
    nodes.update({id: edgeSrc, title: addrs});
  }
}

function removeMapNode(name) {
  var id_ = getId(name);
  if (id_ != -1) {
    nodes.remove({id: id_});
  }
}

function updateMapNode(obj) {
  if ('general' in obj) {
    var id_ = getId(obj.general.hostname);
    if (id_ != -1) {
      var node = nodes.get(id_);
      node.label = obj.general.hostname;
      node.image = DIR + obj.type + '.png';
      node.json = obj;
      nodes.update(node);
    } else {
      addMapNode(obj);
    }
  } else {
    var id_ = getId(obj.name);
    if (id_ != -1) {
      var node = nodes.get(id_);
      if (node.json.type === 'Switch') {
        node.title = 'vlan: ' + obj.id;
        node.json.id = obj.id;
      }
      nodes.update(node);
    } else {
      nodes.add({id: COUNT, label: obj.name, font: {color: 'white'}, image: DIR + 'Switch.png', shape: 'image', json: {type: 'Switch', name: obj.name, id: obj.id}, title: 'vlan: ' + obj.id});
      COUNT++;
    }
  }
}

// data = { nodes: [{}, ...], vlans: [{}, ...] }
function updateMapData(data) {
  if (data != null && 'nodes' in data && data.nodes != 0) {
    data.nodes.forEach(function(obj) {
      updateMapNode(obj);
    });
  } else {
//    alert("No nodes in topology!");
    return;
  }
  if ('vlans' in data && viewerTopo.vlans != 0) {
    data.vlans.forEach(function(obj) {
      updateMapNode(obj);
    });
  }
  if (network != null) {
    var sNodes = network.getSelectedNodes();
    if (sNodes.length > 0) {
      var node = nodes.get(sNodes[0]);
      document.getElementById('nodedetails').innerHTML = '<pre>' + JSON.stringify(node.json, null, 2) + '</pre>';
    }
  }
}

function resetAllNodes() {
  document.getElementById('nodedetails').style.display = 'none';
  nodes.clear();
  edges.clear();
  edges_no_mgmt.clear();
  updateMapData(codeEditor.get());
  network.stabilize();
}

function clearPopUp() {
  document.getElementById('saveButton').onclick = null;
  document.getElementById('cancelButton').onclick = null;
  document.getElementById('network-popUp').style.display = 'none';
  document.getElementById('popUp-table').innerHTML = '';
}

function cancelEdit(callback) {
  clearPopUp();
  callback(null);
}

function saveData(data, callback) {
  var origName;
  if (data.json.type === "Switch") {
    var name = document.getElementById('vlan-name').value;
    var vlan_id = document.getElementById('vlan-id').value;
    origName = data.json.name;
    data.json.name = name;
    data.json.id = vlan_id;
    data.label = name;
    data.title = vlan_id;
  } else {
    var name = document.getElementById('node-hostname').value;
    var type = document.getElementById('node-type').value;
    origName = data.json.general.hostname;
    data.json.type = type;
    data.json.general.hostname = name;
    data.json.hardware.drives[0].image = document.getElementById('node-image').value;
    data.label = name;
    data.image = DIR + type + '.png';
  }
  jsonDisplay.outputPretty(JSON.stringify(data.json));
  updateNode(origName, data.json);
  clearPopUp();
  callback(data);
}

function toJsonView() {
  document.getElementById('container').style.display = 'none';
  document.getElementById('auto').style.display = 'block';
  document.getElementById('reload').style.display = 'none';
  document.getElementById('mgmt').style.display = 'none';
}

function toMapView() {
  document.getElementById('container').style.display = 'flex';
  document.getElementById('auto').style.display = 'none';
  document.getElementById('reload').style.display = 'inline-block';
  document.getElementById('mgmt').style.display = 'inline-block';
}

function toggleMgmtNet() {
  if (document.querySelector('#mgmt').innerHTML == 'Show MGMT Network') {
    network.setData({nodes: nodes, edges: edges});
    network.stabilize();
    document.querySelector('#mgmt').innerHTML = 'Hide MGMT Network';
  }
  else {
    network.setData({nodes: nodes, edges: edges_no_mgmt});
    network.stabilize();
    document.querySelector('#mgmt').innerHTML = 'Show MGMT Network';
  }
}

//
// CREDIT: https://gist.github.com/faffyman/6183311
//
jsonDisplay = {
  jsonstring : '' ,
  outputDivID : 'shpretty',

  outputPretty: function (jsonstring) {
    jsonstring = jsonstring=='' ? jsonDisplay.jsonstring : jsonstring;
    // prettify spacing
    var pretty  = JSON.stringify(JSON.parse(jsonstring),null,2);
    // syntaxhighlight the pretty print version
    shpretty = jsonDisplay.syntaxHighlight(pretty);
    document.getElementById(jsonDisplay.outputDivID).innerHTML = '<pre>' + shpretty + '</pre>';
  },

  syntaxHighlight : function (json) {
    if (typeof json != 'string') {
      json = JSON.stringify(json, undefined, 2);
    }

    json = json.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
    return json.replace(/("(\\u[a-zA-Z0-9]{4}|\\[^u]|[^\\"])*"(\s*:)?|\b(true|false|null)\b|-?\d+(?:\.\d*)?(?:[eE][+\-]?\d+)?)/g, function (match) {
      var cls = 'number';
      if (/^"/.test(match)) {
        if (/:$/.test(match)) {
          cls = 'key';
        } else {
          cls = 'string';
        }
      } else if (/true|false/.test(match)) {
        cls = 'boolean';
      } else if (/null/.test(match)) {
        cls = 'null';
      }
      return '<span class="' + cls + '">' + match + '</span>';
    });
  }
}

//------------------------
//        JSON VIEW
//------------------------
var codeEditor = null, treeEditor = null;

function startJson() {
  var toTree, toCode;
  var codeContainer = document.getElementById('codeEditor');
  var treeContainer = document.getElementById('treeEditor');

  var appOnError = function(err) {
    alert(err.toString());
  };

  codeEditor = new JSONEditor(codeContainer, {mode: "code", onError: appOnError, schema: viewerSchema}, viewerTopo);
  treeEditor = new JSONEditor(treeContainer, {mode: "tree", onError: appOnError, schema: viewerSchema}, viewerTopo);

  toTree = document.getElementById("toTree");
  toTree.onclick = function() {
    this.focus();
    try {
      treeEditor.set(codeEditor.get());
      updateMapData(codeEditor.get());
    } catch (e) {
      appOnError(e);
    }
  };

  toCode = document.getElementById("toCode");
  toCode.onclick = function() {
    this.focus();
    try {
      codeEditor.set(treeEditor.get());
      updateMapData(treeEditor.get());
    } catch (e) {
      appOnError(e);
    }
  };
}

function updateJson() {
  if (treeEditor != null && codeEditor != null) {
    treeEditor.set(viewerTopo);
    codeEditor.set(viewerTopo);
  }
}

function surprise() {
  const elem = document.getElementById("sceptre");
  if (elem.classList.contains('open')) {
    elem.classList.remove('open'); // reset animation
    void elem.offsetWidth; // trigger reflow
    elem.classList.add('close'); // start animation
    document.querySelector('button[data-content="#content7"]').style.display = 'none';
    eventFire(document.querySelector('nav.tabs button:nth-of-type(1)'), 'click');
  } else {
    elem.classList.remove('open'); // reset animation
    elem.classList.remove('close'); // reset animation
    void elem.offsetWidth; // trigger reflow
    elem.classList.add('open'); // start animation
    document.querySelector('button[data-content="#content7"]').style.display = 'inline-block';
    eventFire(document.querySelector('nav.tabs button:nth-of-type(7)'), 'click');
  }
}

document.getElementById('sceptre').addEventListener('click', function (e) {
  if (e.shiftKey) {
    surprise();
  }
});

var console = document.getElementById("content7");

function showConsole() {
//  if (console.src != 'http://localhost:2280/manage/ssh/wetty?pass=wordpass') {
//    console.src = 'http://localhost:2280/manage/ssh/wetty?pass=wordpass';
//  }
  if (console.src != 'http://localhost:3022/manage') {
    console.src = 'http://localhost:3022/manage';
  }
}
