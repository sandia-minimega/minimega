/******************************************************************
Core
******************************************************************/

class JsonCodec extends mxObjectCodec {
    constructor() {
      super((value)=>{});
    }
    encode(value) {
        const xmlDoc = mxUtils.createXmlDocument();
        const newObject = xmlDoc.createElement("Object");
        for(let prop in value) {
          newObject.setAttribute(prop, value[prop]);
        }
        return newObject;
    }
    decode(model) {
      return Object.keys(model.cells).map(
        (iCell)=>{
          const currentCell = model.getCell(iCell);
          return (currentCell.value !== undefined)? currentCell : null;
        }
      ).filter((item)=> (item !== null));
    }
}

class GraphX {
  constructor(container){
      if (!mxClient.isBrowserSupported()) {
          return mxUtils.error('Browser is not supported!', 200, false);
      } 
      // mxEvent.disableContextMenu(container);
      this._graph = new mxGraph(container);
      this._graph.setConnectable(true);
      this._graph.setAllowDanglingEdges(false);
      new mxRubberband(this._graph); // Enables rubberband selection

      this.labelDisplayOveride();
      // this.styling();
  }
  
  labelDisplayOveride() { // Overrides method to provide a cell label in the display
    this._graph.convertValueToString = (cell)=> {
      if (mxUtils.isNode(cell.value)) {
        if (cell.value.nodeName.toLowerCase() === 'object') {
          const name = cell.getAttribute('name', '');
          return name;
        }
      }
      return '';
    };
  }
  
  styling() {
    // Creates the default style for vertices
    let style = [];
    style[mxConstants.STYLE_SHAPE] = mxConstants.SHAPE_RECTANGLE;
    style[mxConstants.STYLE_PERIMETER] = mxPerimeter.RectanglePerimeter;
    style[mxConstants.STYLE_STROKECOLOR] = 'gray';
    style[mxConstants.STYLE_ROUNDED] = true;
    style[mxConstants.STYLE_FILLCOLOR] = '#EEEEEE';
    style[mxConstants.STYLE_GRADIENTCOLOR] = 'white';
    style[mxConstants.STYLE_FONTCOLOR] = 'black';
    style[mxConstants.STYLE_ALIGN] = mxConstants.ALIGN_CENTER;
    style[mxConstants.STYLE_VERTICAL_ALIGN] = mxConstants.ALIGN_MIDDLE;
    style[mxConstants.STYLE_FONTSIZE] = '12';
    style[mxConstants.STYLE_FONTSTYLE] = 1;
    this._graph.getStylesheet().putDefaultVertexStyle(style);

    // Creates the default style for edges
    style = this._graph.getStylesheet().getDefaultEdgeStyle();
    style[mxConstants.STYLE_STROKECOLOR] = '#0C0C0C';
    style[mxConstants.STYLE_LABEL_BACKGROUNDCOLOR] = 'white';
    style[mxConstants.STYLE_EDGE] = mxEdgeStyle.ElbowConnector;
    style[mxConstants.STYLE_ROUNDED] = true;
    style[mxConstants.STYLE_FONTCOLOR] = 'black';
    style[mxConstants.STYLE_FONTSIZE] = '10';
    this._graph.getStylesheet().putDefaultEdgeStyle(style);
  }
  
  getJsonModel(){
      const encoder = new JsonCodec();
      const jsonModel = encoder.decode(this._graph.getModel());
      return {
        "nodes": jsonModel
      }
  }
  
  render(dataModel) {
 
    const jsonEncoder = new JsonCodec();

    this._vertices = [];
    this._edges = {};
    this._dataModelObjects = dataModel;
    console.log(this._dataModelObjects);
    this._dataModelVLANS = dataModel;

    const parent = this._graph.getDefaultParent();
    this._graph.getModel().beginUpdate(); // Adds cells to the model in a single step
    var i = 1; // id iterator;
    try {
      this._dataModelObjects.nodes.map(
        (node, idx)=> {
          if (node.type) {
            node.id = idx + 1;
            if ('mxGraphID' in node){
              node.id = node.mxGraphID; // mxGraphID is custom attribute to persist ID between applications
            }
            node.type = (node.type).toLowerCase();
            node.edge = false;
            node.value = {};
            node.geometry = {
              width: 80,
              height: 80
            };
            if (node.type === 'virtualmachine' && !('mxType' in node)) {
              node.value.type = 'desktop';
            }
            else{
              node.value.type = node.type;
            }
            if (!('style' in node)) {
              switch(node.value.type) {
                case 'desktop':
                  node.style = 'shape=image;html=1;labelBackgroundColor=#ffffff;image=stencils/virtual_machines/desktop_blue_vm.png';
                  break;
                case 'switch':
                  node.style = 'shape=image;html=1;labelBackgroundColor=#ffffff;image=stencils/virtual_machines/switch_blue_vm.png';
                  break;
                case 'router':
                  node.style = 'shape=image;html=1;labelBackgroundColor=#ffffff;image=stencils/virtual_machines/router_blue_vm.png';
                  break;
                case 'server':
                  node.style = 'shape=image;html=1;labelBackgroundColor=#ffffff;image=stencils/virtual_machines/server_blue_vm.png';
                  break;
                case 'firewall':
                  node.style = 'shape=image;html=1;labelBackgroundColor=#ffffff;image=stencils/virtual_machines/firewall_blue_vm.png';
                  break;
                default:
                  node.style = 'shape=image;html=1;labelBackgroundColor=#ffffff;image=stencils/virtual_machines/desktop_blue_vm.png';
              }
            }
            node.value.label = node.general.hostname;
            if ('hardware' in node) {
              node.value.vcpu = node.hardware.vcpu;
              node.value.memory = node.value.memory;
            }
            if ('network' in node) {
              node.value.network = 'vlan-' + node.network.interfaces[0].vlan; // can have multiple interfaces, so need to figure this out
              if ('interfaces' in node.network){
                node.value[node.value.network] = {};
                node.value[node.value.network].interfaces = [];
                node.network.interfaces.forEach(
                  (ifce)=> {
                    node.value[node.value.network].interfaces.push(ifce);
                  }
                );
              }
              if ('routes' in node.network) {
                var i = 0;
                node.value[node.value.network].routes = [];
                node.network.routes.forEach(
                  (route)=> {
                    node.value[node.value.network].routes.push(route);
                  }
                );
              }
              if ('opsf' in node.network) {
                node.value[node.value.network].opsf = node.network.opsf;
              }
              if ('rulesets' in node.network){
                node.value[node.value.network].rulesets = [];
                node.network.rulesets.forEach(
                  (ruleset)=> {
                    node.value[node.value.network].rulesets.push(ruleset);
                  }
                );
              }
            }
            if ('injections' in node) {
              node.value.injections = [];
              node.injections.forEach(
                (injection)=> {
                  node.value.injections.push(injection);
                }
              );
            }
            if ('metadata' in node) {
              node.value.metadata = node.metadata;
            }
           const xmlNode = jsonEncoder.encode(node.value);
           this._vertices[idx] = this._graph.insertVertex(parent, null, xmlNode, node.geometry.x, node.geometry.y, node.geometry.width, node.geometry.height, node.style);
          }
        }
      );
      this._dataModelVLANS.vlans.map(
        (node, idx)=> {
          node.value = {};
          if (!('mxType' in node)) node.value.type = 'diagraming'; // mxType is custom attribute to persist type across applications
          if (!('style' in node)) node.style = 'shape=link;html=1;edge=1';
          node.value.label = node.name;
          node.value.vlan = node.id;
          node.edge = true;
          const xmlNode = jsonEncoder.encode(node.value);
          var sources = [];
          var targets = [];
          console.log(this._vertices);
          this._vertices.forEach(
            (item)=> {
              if ((item.getAttribute('type') === 'switch' || item.getAttribute('type') === 'router') && item.getAttribute('network') === 'vlan-' + node.value.vlan) {
                sources.push(item);
              }
              else if (item.getAttribute('network') === 'vlan-' + node.value.vlan) {
                targets.push(item);
              }
            }
          );
          sources.forEach(
            (source)=> {
              targets.forEach(
                (target)=> {
                  this._edges[(source.id).toString() + (target.id).toString()] = this._graph.insertEdge(parent, null, xmlNode, source, target, node.style);
                }
              );
            }
          );
          console.log(this._edges);
        }
      );

      var layout = new mxFastOrganicLayout(this._graph);
      layout.execute(this._graph.getDefaultParent());

    } finally { 
      this._graph.getModel().endUpdate(); // Updates the display
    }
  }  
}

/******************************************************************
Demo
******************************************************************/

const graphX = new GraphX(document.getElementById('graphContainer'));

document.getElementById('buttons').appendChild(mxUtils.button('to minimega', () => {
  const dataModel = JSON.parse(document.getElementById('from').innerHTML);
  graphX.render(dataModel);
}));

document.getElementById('buttons').appendChild(mxUtils.button('to phenix', () => {
  const jsonNodes = graphX.getJsonModel();
  document.getElementById('to').innerHTML = `<pre>${syntaxHighlight(stringifyWithoutCircular(jsonNodes))}</pre>`;
}));


/******************************************
Utils
********************************************/

function stringifyWithoutCircular(json){
  return JSON.stringify(
      json,
      ( key, value) => {
        if((key === 'parent' || key == 'source' || key == 'target') && value !== null) { 
          return value.id;
        } else if(key === 'value' && value !== null && value.localName) {
          let results = {};
          Object.keys(value.attributes).forEach(
            (attrKey)=>{
              const attribute = value.attributes[attrKey];
              results[attribute.nodeName] = attribute.nodeValue;
            }
          )
          return results;
        }
        return value;
      },
      4
  );
}

function syntaxHighlight(json) {
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
