(function umd(require){
  if ('object' == typeof exports) {
    module.exports = require('1');
  } else if ('function' == typeof define && define.amd) {
    define(function(){ return require('1'); });
  } else {
    this['Grapher'] = require('1');
  }
})((function outer(modules, cache, entries){

  /**
   * Global
   */

  var global = (function(){ return this; })();

  /**
   * Require `name`.
   *
   * @param {String} name
   * @param {Boolean} jumped
   * @api public
   */

  function require(name, jumped){
    if (cache[name]) return cache[name].exports;
    if (modules[name]) return call(name, require);
    throw new Error('cannot find module "' + name + '"');
  }

  /**
   * Call module `id` and cache it.
   *
   * @param {Number} id
   * @param {Function} require
   * @return {Function}
   * @api private
   */

  function call(id, require){
    var m = cache[id] = { exports: {} };
    var mod = modules[id];
    var name = mod[2];
    var fn = mod[0];

    fn.call(m.exports, function(req){
      var dep = modules[id][1][req];
      return require(dep ? dep : req);
    }, m, m.exports, outer, modules, cache, entries);

    // expose as `name`.
    if (name) cache[name] = cache[id];

    return cache[id].exports;
  }

  /**
   * Require all entries exposing them on global if needed.
   */

  for (var id in entries) {
    if (entries[id]) {
      global[entries[id]] = require(id);
    } else {
      require(id);
    }
  }

  /**
   * Duo flag.
   */

  require.duo = true;

  /**
   * Expose cache.
   */

  require.cache = cache;

  /**
   * Expose modules
   */

  require.modules = modules;

  /**
   * Return newest require.
   */

   return require;
})({
1: [function(require, module, exports) {
// Ayasdi Inc. Copyright 2014
// Grapher.js may be freely distributed under the Apache 2.0 license

;(function () {
  
/**
  * Grapher
  * =======
  * WebGL network grapher rendering with PIXI
  */
  function Grapher () {
    this.initialize.apply(this, arguments);
    return this;
  }

/**
  * Helpers and Renderers
  * =====================
  * Load helpers and renderers.
  */
  var WebGLRenderer = Grapher.WebGLRenderer = require('./renderers/gl/renderer.js'),
      CanvasRenderer = Grapher.CanvasRenderer = require('./renderers/canvas/renderer.js'),
      Color = Grapher.Color = require('./helpers/color.js'),
      Link = Grapher.Link = require('./helpers/link.js'),
      Node = Grapher.Node = require('./helpers/node.js'),
      u = Grapher.utils = require('./helpers/utilities.js');

  Grapher.prototype = {};

  /**
    * grapher.initialize
    * ------------------
    * 
    * Initialize is called when a grapher instance is created:
    *     
    *     var grapher = new Grapher(width, height, options);
    *
    */
  Grapher.prototype.initialize = function (o) {
    if (!o) o = {};
    
    // Extend default properties with options
    this.props = u.extend({
      color: 0x222222,
      scale: 1,
      translate: [0, 0],
      resolution: window.devicePixelRatio || 1
    }, o);

    if (!o.canvas) this.props.canvas = document.createElement('canvas');
    this.canvas = this.props.canvas;

    var webGL = this._getWebGL();
    if (webGL) {
      this.props.webGL = webGL;
      this.props.canvas.addEventListener('webglcontextlost', function (e) { this._onContextLost(e); }.bind(this));
      this.props.canvas.addEventListener('webglcontextrestored', function (e) { this._onContextRestored(e); }.bind(this));
    }

    // Renderer and view
    this.renderer =  webGL ? new WebGLRenderer(this.props) : new CanvasRenderer(this.props);
    this.rendered = false;

    // Sprite array
    this.links = [];
    this.nodes = [];

    this.renderer.setLinks(this.links);
    this.renderer.setNodes(this.nodes);

    // Indices that will update
    this.willUpdate = {};
    this.updateAll = {};
    this._clearUpdateQueue();

    // Bind some updaters
    this._updateLink = u.bind(this._updateLink, this);
    this._updateNode = u.bind(this._updateNode, this);
    this._updateLinkByIndex = u.bind(this._updateLinkByIndex, this);
    this._updateNodeByIndex = u.bind(this._updateNodeByIndex, this);
    this.animate = u.bind(this.animate, this);

    // Event Handlers
    this.handlers = {};

    // Do any additional setup
    u.eachKey(o, this.set, this);
  };

  /**
    * grapher.set
    * ------------------
    * 
    * General setter for a grapher's properties.
    *
    *     grapher.set(1, 'scale');
    */
  Grapher.prototype.set = function (val, key) {
    var setter = this[key];
    if (setter && u.isFunction(setter))
      return setter.call(this, val);
  };

  /**
    * grapher.on
    * ------------------
    * 
    * Add a listener to a grapher event. Only one listener can be bound to an
    * event at this time. Available events:
    *
    *   * mousedown
    *   * mouseover
    *   * mouseup
    */
  Grapher.prototype.on = function (event, fn) {
    this.handlers[event] = this.handlers[event] || [];
    this.handlers[event].push(fn);
    this.canvas.addEventListener(event, fn, false);
    return this;
  };

  /**
    * grapher.off
    * ------------------
    * 
    * Remove a listener from an event, or all listeners from an event if fn is not specified.
    */
  Grapher.prototype.off = function (event, fn) {
    var removeHandler = u.bind(function (fn) {
      var i = u.indexOf(this.handlers[event], fn);
      if (i > -1) this.handlers[event].splice(i, 1);
      this.canvas.removeEventListener(event, fn, false);
    }, this);

    if (fn && this.handlers[event]) removeHandler(fn);
    else if (u.isUndefined(fn) && this.handlers[event]) u.each(this.handlers[event], removeHandler);

    return this;
  };

  /**
    * grapher.data
    * ------------------
    * 
    * Accepts network data in the form:
    *
    *     {
    *       nodes: [{x: 0, y: 0, r: 20, color: (swatch or hex/rgb)}, ... ],
    *       links: [{from: 0, to: 1, color: (swatch or hex/rgb)}, ... ]
    *     }
    */
  Grapher.prototype.data = function (data) {
    if (u.isUndefined(data)) return this.props.data;

    this.props.data = data;
    this.exit();
    this.enter();
    this.update();

    return this;
  };

  /**
    * grapher.enter
    * ------------------
    * 
    * Creates node and link sprites to match the number of nodes and links in the
    * data.
    */
  Grapher.prototype.enter = function () {
    var data = this.data();
    if (this.links.length < data.links.length) {
      var links = data.links.slice(this.links.length, data.links.length);
      u.eachPop(links, u.bind(function () { this.links.push(new Link()); }, this));
    }

    if (this.nodes.length < data.nodes.length) {
      var nodes = data.nodes.slice(this.nodes.length, data.nodes.length);
      u.eachPop(nodes, u.bind(function () { this.nodes.push(new Node()); }, this));
    }

    return this;
  };

  /**
    * grapher.exit
    * ------------------
    * 
    * Removes node and link sprites to match the number of nodes and links in the
    * data.
    */
  Grapher.prototype.exit = function () {
    var data = this.data(),
        exiting = [];

    if (data.links.length < this.links.length) {
      this.links.splice(data.links.length, this.links.length - data.links.length);
    }
    if (data.nodes.length < this.nodes.length) {
      this.nodes.splice(data.nodes.length, this.nodes.length - data.nodes.length);
    }

    return this;
  };

  /**
    * grapher.update
    * ------------------
    * 
    * Add nodes and/or links to the update queue by index. Passing in no arguments will 
    * add all nodes and links to the update queue. Node and link sprites in the update
    * queue are updated at the time of rendering.
    *
    *     grapher.update(); // updates all nodes and links
    *     grapher.update('links'); // updates only links
    *     grapher.update('nodes', 0, 4); // updates nodes indices 0 to 3 (4 is not inclusive)
    *     grapher.update('links', [0, 1, 2, 6, 32]); // updates links indexed by the indices
    */
  Grapher.prototype.update = function (type, start, end) {
    var indices;
    if (u.isArray(start)) indices = start;
    else if (u.isNumber(start) && u.isNumber(end)) indices = u.range(start, end);

    if (u.isArray(indices)) {
      this._addToUpdateQueue(type, indices);
      if (type === NODES) this._addToUpdateQueue(LINKS, this._findLinks(indices));
    } else {
      if (type !== NODES) this.updateAll.links = true;
      if (type !== LINKS) this.updateAll.nodes = true;
    }
    return this;
  };

  /**
    * grapher.updateNode
    * ------------------
    * 
    * Add an individual node to the update queue. Optionally pass in a boolean to
    * specify whether or not to also add links connected with the node to the update queue.
    */
  Grapher.prototype.updateNode = function (index, willUpdateLinks) {
    this._addToUpdateQueue(NODES, [index]);
    if (willUpdateLinks) this._addToUpdateQueue(LINKS, this._findLinks([index]));
    return this;
  };

  /**
    * grapher.updateLink
    * ------------------
    * 
    * Add an individual link to the update queue.
    */
  Grapher.prototype.updateLink = function (index) {
    this._addToUpdateQueue(LINKS, [index]);
    return this;
  };

  /**
    * grapher.render
    * ------------------
    * 
    * Updates each sprite and renders the network.
    */
  Grapher.prototype.render = function () {
    this.rendered = true;
    this._update();
    this.renderer.render();
    return this;
  };

  /**
    * grapher.animate
    * ------------------
    * 
    * Calls render in a requestAnimationFrame loop.
    */
  Grapher.prototype.animate = function (time) {
    this.render();
    this.currentFrame = requestAnimationFrame(this.animate);
  };

  /**
    * grapher.play
    * ------------------
    * 
    * Starts the animate loop.
    */
  Grapher.prototype.play = function () {
    this.currentFrame = requestAnimationFrame(this.animate);
    return this;
  };

  /**
    * grapher.pause
    * ------------------
    * 
    * Pauses the animate loop.
    */
  Grapher.prototype.pause = function () {
    if (this.currentFrame) cancelAnimationFrame(this.currentFrame);
    this.currentFrame = null;
    return this;
  };

  /**
    * grapher.resize
    * ------------------
    * 
    * Resize the grapher view.
    */
  Grapher.prototype.resize = function (width, height) {
    this.renderer.resize(width, height);
    return this;
  };

  /**
    * grapher.width
    * ------------------
    * 
    * Specify or retrieve the width.
    */
  Grapher.prototype.width = function (width) {
    if (u.isUndefined(width)) return this.canvas.clientWidth;
    this.resize(width, null);
    return this;
  };

   /**
    * grapher.height
    * ------------------
    * 
    * Specify or retrieve the height.
    */
  Grapher.prototype.height = function (height) {
    if (u.isUndefined(height)) return this.canvas.clientHeight;
    this.resize(null, height);
    return this;
  };

  /**
    * grapher.transform
    * ------------------
    * 
    * Set the scale and translate as an object.
    * If no arguments are passed in, returns the current transform object.
    */
  Grapher.prototype.transform = function (transform) {
    if (u.isUndefined(transform))
      return {scale: this.props.scale, translate: this.props.translate};

    this.scale(transform.scale);
    this.translate(transform.translate);
    return this;
  };

  /**
    * grapher.scale
    * ------------------
    * 
    * Set the scale.
    * If no arguments are passed in, returns the current scale.
    */
  Grapher.prototype.scale = function (scale) {
    if (u.isUndefined(scale)) return this.props.scale;
    if (u.isNumber(scale)) this.props.scale = scale;
    this.updateTransform = true;
    return this;
  };

  /**
    * grapher.translate
    * ------------------
    * 
    * Set the translate.
    * If no arguments are passed in, returns the current translate.
    */
  Grapher.prototype.translate = function (translate) {
    if (u.isUndefined(translate)) return this.props.translate;
    if (u.isArray(translate)) this.props.translate = translate;
    this.updateTransform = true;
    return this;
  };

  /**
    * grapher.color
    * ------------------
    * 
    * Set the default color of nodes and links.
    * If no arguments are passed in, returns the current default color.
    */
  Grapher.prototype.color = function (color) {
    if (u.isUndefined(color)) return this.props.color;
    this.props.color = Color.parse(color);
    return this;
  };

  /**
    * grapher.getDataPosition
    * ------------------
    * 
    * Returns data space coordinates given display coordinates.
    * If a single argument passed in, function considers first argument an object with x and y props.
    */
  Grapher.prototype.getDataPosition = function (x, y) {
    var xCoord = u.isUndefined(y) ? x.x : x;
    var yCoord = u.isUndefined(y) ? x.y : y;
    x = this.renderer.untransformX(xCoord);
    y = this.renderer.untransformY(yCoord);
    return {x: x, y: y};
  };

  /**
  * grapher.getDisplayPosition
  * ------------------
  * 
  * Returns display space coordinates given data coordinates.
  * If a single argument passed in, function considers first argument an object with x and y props.
  */
  Grapher.prototype.getDisplayPosition = function (x, y) {
    var xCoord = u.isUndefined(y) ? x.x : x;
    var yCoord = u.isUndefined(y) ? x.y : y;
    x = this.renderer.transformX(xCoord);
    y = this.renderer.transformY(yCoord);
    return {x: x, y: y};
  };

/**
  * Private Functions
  * =================
  * 
  */

  /**
    * grapher._addToUpdateQueue
    * -------------------
    * 
    * Add indices to the nodes or links update queue.
    *
    */
  Grapher.prototype._addToUpdateQueue = function (type, indices) {
    var willUpdate = type === NODES ? this.willUpdate.nodes : this.willUpdate.links,
        updateAll = type === NODES ? this.updateAll.nodes : this.updateAll.links,
        spriteSet = type === NODES ? this.nodes : this.links;

    var insert = function (n) { u.uniqueInsert(willUpdate, n); };
    if (!updateAll && u.isArray(indices)) u.each(indices, insert, this);

    updateAll = updateAll || willUpdate.length >= spriteSet.length;

    if (type === NODES) this.updateAll.nodes = updateAll;
    else this.updateAll.links = updateAll;
  };

  /**
    * grapher._clearUpdateQueue
    * -------------------
    * 
    * Clear the update queue.
    *
    */
  Grapher.prototype._clearUpdateQueue = function () {
    this.willUpdate.links = [];
    this.willUpdate.nodes = [];
    this.updateAll.links = false;
    this.updateAll.nodes = false;
    this.updateTransform = false;
  };

  /**
    * grapher._update
    * -------------------
    * 
    * Update nodes and links in the update queue.
    *
    */
  Grapher.prototype._update = function () {
    var updatingLinks = this.willUpdate.links,
        updatingNodes = this.willUpdate.nodes,
        i;

    if (this.updateAll.links) u.each(this.links, this._updateLink);
    else if (updatingLinks && updatingLinks.length) u.eachPop(updatingLinks, this._updateLinkByIndex);

    if (this.updateAll.nodes) u.each(this.nodes, this._updateNode);
    else if (updatingNodes && updatingNodes.length) u.eachPop(updatingNodes, this._updateNodeByIndex);

    if (this.updateTransform) {
      this.renderer.setScale(this.props.scale);
      this.renderer.setTranslate(this.props.translate);
    }

    this._clearUpdateQueue();
  };

  Grapher.prototype._updateLink = function (link, i) {
    var data = this.data(),
        l = data.links[i],
        from = data.nodes[l.from],
        to = data.nodes[l.to];

    var color = !u.isUndefined(l.color) ? this._findColor(l.color) :
        Color.interpolate(this._findColor(from.color), this._findColor(to.color));

    link.update(from.x, from.y, to.x, to.y, color);
  };

  Grapher.prototype._updateNode = function (node, i) {
    var n = this.data().nodes[i];
    node.update(n.x, n.y, n.r, this._findColor(n.color));
  };

  Grapher.prototype._updateNodeByIndex = function (i) { this._updateNode(this.nodes[i], i); };

  Grapher.prototype._updateLinkByIndex = function (i) { this._updateLink(this.links[i], i); };

  /**
    * grapher._findLinks
    * -------------------
    * 
    * Search for links connected to the node indices provided.
    *
    * isLinked is a helper function that returns true if a link is
    * connected to a node in indices.
    */
  var isLinked = function (indices, l) {
    var i, len = indices.length, flag = false;
    for (i = 0; i < len; i++) {
      if (l.to == indices[i] || l.from == indices[i]) {
        flag = true;
        break;
      }
    }
    return flag;
  };

  Grapher.prototype._findLinks = function (indices) {
    var links = this.data().links,
        i, numLinks = links.length,
        updatingLinks = [];

    for (i = 0; i < numLinks; i++) {
      if (isLinked(indices, links[i])) updatingLinks.push(i);
    }

    return updatingLinks;
  };

  /**
    * grapher._findColor
    * -------------------
    * 
    * Search for a color whether it's defined by palette index, string,
    * integer.
    */
  Grapher.prototype._findColor = function (c) {
    var color = Color.parse(c);

    // if color is still not set, use the default
    if (u.isNaN(color)) color = this.color();
    return color;
  };

  /**
    * grapher._getWebGL
    * -------------------
    * 
    *get webGL context if available
    *
    */
  Grapher.prototype._getWebGL = function () {
    var gl = null;
    try { gl = this.canvas.getContext("webgl") || this.canvas.getContext("experimental-webgl"); }
    catch (x) { gl = null; }
    return gl;
  };

 /**
    * grapher._onContextLost
    * ----------------------
    * 
    * Handle context lost.
    *
    */
  Grapher.prototype._onContextLost = function (e) {
    e.preventDefault();
    if (this.currentFrame) cancelAnimationFrame(this.currentFrame);
  };

  /**
    * grapher._onContextRestored
    * --------------------------
    * 
    * Handle context restored.
    *
    */
  Grapher.prototype._onContextRestored = function (e) {
    var webGL = this._getWebGL();
    this.renderer.initGL(webGL);
    if (this.currentFrame) this.play(); // Play the graph if it was running.
    else if (this.rendered) this.render();
  };


/**
  * Grapher Static Properties
  * =========================
  */
  var NODES = Grapher.NODES = 'nodes';
  var LINKS = Grapher.LINKS = 'links';

  if (module && module.exports) module.exports = Grapher;
})();

}, {"./renderers/gl/renderer.js":2,"./renderers/canvas/renderer.js":3,"./helpers/color.js":4,"./helpers/link.js":5,"./helpers/node.js":6,"./helpers/utilities.js":7}],
2: [function(require, module, exports) {
;(function () {
  var LinkVertexShaderSource = require('./shaders/link.vert'),
      LinkFragmentShaderSource = require('./shaders/link.frag'),
      NodeVertexShaderSource = require('./shaders/node.vert'),
      NodeFragmentShaderSource = require('./shaders/node.frag'),
      Renderer = require('../renderer.js');

  var WebGLRenderer = Renderer.extend({
    init: function (o) {
      this.initGL(o.webGL);
      this._super(o);

      this.NODE_ATTRIBUTES = 6;
      this.LINKS_ATTRIBUTES = 3;
    },

    initGL: function (gl) {
      this.gl = gl;

      this.linksProgram = this.initShaders(LinkVertexShaderSource, LinkFragmentShaderSource);
      this.nodesProgram = this.initShaders(NodeVertexShaderSource, NodeFragmentShaderSource);

      this.gl.linkProgram(this.linksProgram);
      this.gl.linkProgram(this.nodesProgram);

      this.gl.blendFuncSeparate(this.gl.SRC_ALPHA, this.gl.ONE_MINUS_SRC_ALPHA, this.gl.ONE, this.gl.ONE_MINUS_SRC_ALPHA);
      this.gl.enable(this.gl.BLEND);
    },

    initShaders: function (vertexShaderSource, fragmentShaderSource) {
      var vertexShader = this.getShaders(this.gl.VERTEX_SHADER, vertexShaderSource);
      var fragmentShader = this.getShaders(this.gl.FRAGMENT_SHADER, fragmentShaderSource);
      var shaderProgram = this.gl.createProgram();
      this.gl.attachShader(shaderProgram, vertexShader);
      this.gl.attachShader(shaderProgram, fragmentShader);
      return shaderProgram;
    },

    getShaders: function (type, source) {
      var shader = this.gl.createShader(type);
      this.gl.shaderSource(shader, source);
      this.gl.compileShader(shader);
      return shader;
    },

    updateNodesBuffer: function () {
      var j = 0;
      this.nodes = [];
      for (var i = 0; i < this.nodeObjects.length; i++) {
        var node = this.nodeObjects[i];
        var cx = this.transformX(node.x) * this.resolution;
        var cy = this.transformY(node.y) * this.resolution;
        // adding one px to keep shader area big enough for antialiasing pixesls
        var r = node.r * Math.abs(this.scale * this.resolution) + 1;
        var color = node.color;


        this.nodes[j++] = (cx - r);
        this.nodes[j++] = (cy - r);
        this.nodes[j++] = color;
        this.nodes[j++] = cx;
        this.nodes[j++] = cy;
        this.nodes[j++] = r;

        this.nodes[j++] = (cx + (1 + Math.sqrt(2))*r);
        this.nodes[j++] = cy - r;
        this.nodes[j++] = color;
        this.nodes[j++] = cx;
        this.nodes[j++] = cy;
        this.nodes[j++] = r;

        this.nodes[j++] = (cx - r);
        this.nodes[j++] = (cy + (1 + Math.sqrt(2))*r);
        this.nodes[j++] = color;
        this.nodes[j++] = cx;
        this.nodes[j++] = cy;
        this.nodes[j++] = r;
      }
    },

    updateLinksBuffer: function () {
      var j = 0;
      this.links = [];
      for (var i = 0; i < this.linkObjects.length; i++) {
        var link = this.linkObjects[i];
        var x1 = this.transformX(link.x1) * this.resolution;
        var y1 = this.transformY(link.y1) * this.resolution;
        var x2 = this.transformX(link.x2) * this.resolution;
        var y2 = this.transformY(link.y2) * this.resolution;
        var color = link.color;

        this.links[j++] = x1;
        this.links[j++] = y1;
        this.links[j++] = color;

        this.links[j++] = x2;
        this.links[j++] = y2;
        this.links[j++] = color;
      }
    },

    resize: function (width, height) {
      this._super(width, height);
      this.gl.viewport(0, 0, this.gl.drawingBufferWidth, this.gl.drawingBufferHeight);
    },

    render: function () {
      this.gl.clear(this.gl.COLOR_BUFFER_BIT | this.gl.DEPTH_BUFFER_BIT);

      this.resize();
      this.updateNodesBuffer();
      this.updateLinksBuffer();
      this.renderLinks(); // links have to be rendered first because of blending;
      this.renderNodes();
    },

    renderLinks: function () {
      var program = this.linksProgram;
      this.gl.useProgram(program);

      var linksBuffer = new Float32Array(this.links);
      var buffer = this.gl.createBuffer();

      this.gl.bindBuffer(this.gl.ARRAY_BUFFER, buffer);
      this.gl.bufferData(this.gl.ARRAY_BUFFER, linksBuffer, this.gl.STATIC_DRAW);

      var resolutionLocation = this.gl.getUniformLocation(program, 'u_resolution');
      this.gl.uniform2f(resolutionLocation, this.canvas.width, this.canvas.height);

      var positionLocation = this.gl.getAttribLocation(program, 'a_position');
      var colorLocation = this.gl.getAttribLocation(program, 'a_color');
      
      this.gl.enableVertexAttribArray(positionLocation);
      this.gl.enableVertexAttribArray(colorLocation);

      this.gl.vertexAttribPointer(positionLocation, 2, this.gl.FLOAT, false, this.LINKS_ATTRIBUTES * Float32Array.BYTES_PER_ELEMENT, 0);
      this.gl.vertexAttribPointer(colorLocation, 1, this.gl.FLOAT, false, this.LINKS_ATTRIBUTES * Float32Array.BYTES_PER_ELEMENT, 8);

      var lineWidthRange = this.gl.getParameter(this.gl.ALIASED_LINE_WIDTH_RANGE), // ex [1,10] 
          lineWidth = this.lineWidth * Math.abs(this.scale * this.resolution),
          lineWidthInRange = Math.min(Math.max(lineWidth, lineWidthRange[0]), lineWidthRange[1]);

      this.gl.lineWidth(lineWidthInRange);
      this.gl.drawArrays(this.gl.LINES, 0, this.links.length/this.LINKS_ATTRIBUTES);
    },

    renderNodes: function () {
      var program = this.nodesProgram;
      this.gl.useProgram(program);

      var nodesBuffer = new Float32Array(this.nodes);
      var buffer = this.gl.createBuffer();

      this.gl.bindBuffer(this.gl.ARRAY_BUFFER, buffer);
      this.gl.bufferData(this.gl.ARRAY_BUFFER, nodesBuffer, this.gl.STATIC_DRAW);

      var resolutionLocation = this.gl.getUniformLocation(program, 'u_resolution');
      this.gl.uniform2f(resolutionLocation, this.canvas.width, this.canvas.height);

      var positionLocation = this.gl.getAttribLocation(program, 'a_position');
      var colorLocation = this.gl.getAttribLocation(program, 'a_color');
      var centerLocation = this.gl.getAttribLocation(program, 'a_center');
      var radiusLocation = this.gl.getAttribLocation(program, 'a_radius');
      
      this.gl.enableVertexAttribArray(positionLocation);
      this.gl.enableVertexAttribArray(colorLocation);
      this.gl.enableVertexAttribArray(centerLocation);
      this.gl.enableVertexAttribArray(radiusLocation);

      this.gl.vertexAttribPointer(positionLocation, 2, this.gl.FLOAT, false, this.NODE_ATTRIBUTES * Float32Array.BYTES_PER_ELEMENT, 0);
      this.gl.vertexAttribPointer(colorLocation, 1, this.gl.FLOAT, false, this.NODE_ATTRIBUTES * Float32Array.BYTES_PER_ELEMENT, 8);
      this.gl.vertexAttribPointer(centerLocation, 2, this.gl.FLOAT, false, this.NODE_ATTRIBUTES * Float32Array.BYTES_PER_ELEMENT, 12);
      this.gl.vertexAttribPointer(radiusLocation, 1, this.gl.FLOAT, false, this.NODE_ATTRIBUTES * Float32Array.BYTES_PER_ELEMENT, 20);

      this.gl.drawArrays(this.gl.TRIANGLES, 0, this.nodes.length/this.NODE_ATTRIBUTES);
    }
  });

  if (module && module.exports) module.exports = WebGLRenderer;
})();

}, {"./shaders/link.vert":8,"./shaders/link.frag":9,"./shaders/node.vert":10,"./shaders/node.frag":11,"../renderer.js":12}],
8: [function(require, module, exports) {
module.exports = 'uniform vec2 u_resolution;\nattribute vec2 a_position;\nattribute float a_color;\nvarying vec4 color;\nvarying vec2 position;\nvarying vec2 resolution;\nvoid main() {\n  vec2 clipspace = a_position / u_resolution * 2.0 - 1.0;\n  gl_Position = vec4(clipspace * vec2(1, -1), 0, 1);\n  float c = a_color;\n  color.b = mod(c, 256.0); c = floor(c / 256.0);\n  color.g = mod(c, 256.0); c = floor(c / 256.0);\n  color.r = mod(c, 256.0); c = floor(c / 256.0); color /= 255.0;\n  color.a = 1.0;\n}\n';
}, {}],
9: [function(require, module, exports) {
module.exports = 'precision mediump float;\nvarying vec4 color;\nvoid main() {\n  gl_FragColor = color;\n}\n';
}, {}],
10: [function(require, module, exports) {
module.exports = 'uniform vec2 u_resolution;\nattribute vec2 a_position;\nattribute float a_color;\nattribute vec2 a_center;\nattribute float a_radius;\nvarying vec3 rgb;\nvarying vec2 center;\nvarying vec2 resolution;\nvarying float radius;\nvoid main() {\n  vec2 clipspace = a_position / u_resolution * 2.0 - 1.0;\n  gl_Position = vec4(clipspace * vec2(1, -1), 0, 1);\n  float c = a_color;\n  rgb.b = mod(c, 256.0); c = floor(c / 256.0);\n  rgb.g = mod(c, 256.0); c = floor(c / 256.0);\n  rgb.r = mod(c, 256.0); c = floor(c / 256.0); rgb /= 255.0;\n  radius = a_radius - 1.0 ;\n  center = a_center;\n  resolution = u_resolution;\n}\n';
}, {}],
11: [function(require, module, exports) {
module.exports = 'precision mediump float;\nvarying vec3 rgb;\nvarying vec2 center;\nvarying vec2 resolution;\nvarying float radius;\nvoid main() {\n  vec4 color0 = vec4(0.0, 0.0, 0.0, 0.0);\n  float x = gl_FragCoord.x;\n  float y = resolution[1] - gl_FragCoord.y;\n  float dx = center[0] - x;\n  float dy = center[1] - y;\n  float distance = sqrt(dx*dx + dy*dy);\n  float diff = distance - radius;\n  if ( diff < 0.0 ) \n    gl_FragColor = vec4(rgb, 1.0);\n  else if ( diff >= 0.0 && diff <= 1.0 )\n    gl_FragColor = vec4(rgb, 1.0 - diff);\n  else \n    gl_FragColor = color0;\n}\n';
}, {}],
12: [function(require, module, exports) {
;(function () {

  var Renderer = function () {
    if ( !initializing && this.init )
      this.init.apply(this, arguments);
    return this;
  };

  Renderer.prototype = {
    init: function (o) {
      this.canvas = o.canvas;
      this.lineWidth = o.lineWidth || 2;
      this.resolution = o.resolution || 1;
      this.scale = o.scale;
      this.translate = o.translate;

      this.resize();
    },
    setNodes: function (nodes) { this.nodeObjects = nodes; },
    setLinks: function (links) { this.linkObjects = links; },
    setScale: function (scale) { this.scale = scale; },
    setTranslate: function (translate) { this.translate = translate; },
    transformX: function (x) { return x * this.scale + this.translate[0]; },
    transformY: function (y) { return y * this.scale + this.translate[1]; },
    untransformX: function (x) { return (x - this.translate[0]) / this.scale; },
    untransformY: function (y) { return (y - this.translate[1]) / this.scale; },
    resize: function (width, height) {
      var displayWidth  = (width || this.canvas.clientWidth) * this.resolution;
      var displayHeight = (height || this.canvas.clientHeight) * this.resolution;

      if (this.canvas.width != displayWidth) this.canvas.width  = displayWidth;
      if (this.canvas.height != displayHeight) this.canvas.height = displayHeight;
    }
  };

  var initializing = false;

  Renderer.extend = function (prop) {
    var _super = this.prototype;

    initializing = true;
    var prototype = new this();
    initializing = false;

    prototype._super = this.prototype;
    for (var name in prop) {
      prototype[name] = typeof prop[name] == "function" &&
        typeof _super[name] == "function" && /\b_super\b/.test(prop[name]) ?
        (function(name, fn){
          return function() {
            var tmp = this._super;
           
            // Add a new ._super() method that is the same method
            // but on the super-class
            this._super = _super[name];
           
            // The method only need to be bound temporarily, so we
            // remove it when we're done executing
            var ret = fn.apply(this, arguments);
            this._super = tmp;
           
            return ret;
          };
        })(name, prop[name]) :
        prop[name];
    }

    // The dummy class constructor
    function Renderer () {
      // All construction is actually done in the init method
      if ( !initializing && this.init )
        this.init.apply(this, arguments);
    }
   
    // Populate our constructed prototype object
    Renderer.prototype = prototype;
   
    // Enforce the constructor to be what we expect
    Renderer.prototype.constructor = Renderer;
 
    // And make this class extendable
    Renderer.extend = arguments.callee;
   
    return Renderer;
  };

  if (module && module.exports) module.exports = Renderer;
})();

}, {}],
3: [function(require, module, exports) {
;(function () {

  var Renderer = require('../renderer.js');
  var Color = require('../../helpers/color.js');
  
  var CanvasRenderer = Renderer.extend({
    init: function (o) {
      this._super(o);
      this.context = this.canvas.getContext('2d');
    },

    render: function () {
      this.resize();
      this.context.clearRect(0, 0, this.canvas.width, this.canvas.height);
      this.renderLinks();
      this.renderNodes();
    },

    renderNodes: function () {
      for (var i = 0 ; i < this.nodeObjects.length; i ++) {
        var node = this.nodeObjects[i];
        var cx = this.transformX(node.x) * this.resolution;
        var cy = this.transformY(node.y) * this.resolution;
        var r = node.r * Math.abs(this.scale * this.resolution);

        this.context.beginPath();
        this.context.arc(cx, cy, r, 0, 2 * Math.PI, false);
        this.context.fillStyle = Color.toRgb(node.color);
        this.context.fill();
      }
    },

    renderLinks: function () {
      for (var i = 0 ; i < this.linkObjects.length; i ++) {
        var link = this.linkObjects[i];
        var x1 = this.transformX(link.x1) * this.resolution;
        var y1 = this.transformY(link.y1) * this.resolution;
        var x2 = this.transformX(link.x2) * this.resolution;
        var y2 = this.transformY(link.y2) * this.resolution;

        this.context.beginPath();
        this.context.moveTo(x1, y1);
        this.context.lineTo(x2, y2);
        this.context.lineWidth = this.lineWidth * Math.abs(this.scale * this.resolution);

        this.context.strokeStyle = Color.toRgb(link.color);
        this.context.stroke();
      }
    }
  });
  
  if (module && module.exports) module.exports = CanvasRenderer;
})();

}, {"../renderer.js":12,"../../helpers/color.js":4}],
4: [function(require, module, exports) {
// Ayasdi Inc. Copyright 2014
// Color.js may be freely distributed under the Apache 2.0 license

var Color = module.exports = {
  hexToRgb: hexToRgb,
  rgbToHex: rgbToHex,
  toRgb: toRgb,
  interpolate: interpolate,
  parse: parse
};

function hexToRgb (hex) {
  return {r: (hex >> 16) & 0xff, g: (hex >> 8) & 0xff, b: hex & 0xff};
};

function rgbToHex (r, g, b) {
  return r << 16 | g << 8 | b;
};

function interpolate (a, b, amt) {
  amt = amt === undefined ? 0.5 : amt;
  var colorA = hexToRgb(a),
      colorB = hexToRgb(b),
      interpolated = {
        r: colorA.r + (colorB.r - colorA.r) * amt,
        g: colorA.g + (colorB.g - colorA.g) * amt,
        b: colorA.b + (colorB.b - colorA.b) * amt
      };
  return rgbToHex(interpolated.r, interpolated.g, interpolated.b);
};

function parse (c) {
  var color = parseInt(c);
  if (typeof c === 'string') {
    if (c.split('#').length > 1) { // hex format '#ffffff'
      color = parseInt(c.replace('#', ''), 16);
    }

    else if (c.split('rgb(').length > 1) { // rgb format 'rgb(255, 255, 255)'
      var rgb = c.substring(4, c.length-1).replace(/ /g, '').split(',');
      color = rgbToHex(rgb[0], rgb[1], rgb[2]);
    }
  }
  return color;
};

function toRgb (intColor) {
  var r = (intColor >> 16) & 255;
  var g = (intColor >> 8) & 255;
  var b = intColor & 255;

  return 'rgb(' + r + ', ' + g + ', ' + b + ')';
};
}, {}],
5: [function(require, module, exports) {
;(function () {
  function Link () {
    this.x1 = 0;
    this.y1 = 0;
    this.x2 = 0;
    this.y2 = 0;
    this.color = 0;
    return this;
  }

  Link.prototype.update = function (x1, y1, x2, y2, color) {
    this.x1 = x1;
    this.y1 = y1;
    this.x2 = x2;
    this.y2 = y2;
    this.color = color;
    return this;
  };

  if (module && module.exports) module.exports = Link;
})();

}, {}],
6: [function(require, module, exports) {
;(function () {
  function Node () {
    this.x = 0;
    this.y = 0;
    this.r = 10;
    this.color = 0;
    return this;
  }

  Node.prototype.update = function (x, y, r, color) {
    this.x = x;
    this.y = y;
    this.r = r;
    this.color = color;
    return this;
  };

  if (module && module.exports) module.exports = Node;
})();

}, {}],
7: [function(require, module, exports) {
/**
 * Utilities
 * =========
 *
 * Various utility functions
 */
var Utilities = module.exports = {
  each: each,
  eachPop: eachPop,
  eachKey: eachKey,
  map: map,
  clean: clean,
  range: range,
  sortedIndex: sortedIndex,
  indexOf: indexOf,
  uniqueInsert: uniqueInsert,
  extend: extend,
  bind: bind,
  noop: noop,
  isUndefined: isUndefined,
  isFunction: isFunction,
  isObject: isObject,
  isArray: Array.isArray,
  isNumber: isNumber,
  isNaN: isNaN
};

/**
 * noop
 * -----
 *
 * A function that does nothing.
 */
function noop () {}

/**
 * each
 * -----
 *
 * Perform an operation on each element in an array.
 *
 *     var arr = [1, 2, 3];
 *     u.each(arr, fn);
 */
function each (arr, fn, ctx) {
  fn = bind(fn, ctx);
  var i = arr.length;
  while (--i > -1) {
    fn(arr[i], i);
  }
  return arr;
}

/**
 * eachPop
 * -------
 *
 * Perform a function on each element in an array. Faster than each, but won't pass index and the
 * array will be cleared.
 *
 *     u.eachPop([1, 2, 3], fn);
 */
function eachPop (arr, fn, ctx) {
  fn = bind(fn, ctx);
  while (arr.length) {
    fn(arr.pop());
  }
  return arr;
}

/**
 * eachKey
 * -------
 *
 * Perform a function on each property in an object.
 *
 *     var obj = {foo: 0, bar: 0};
 *     u.eachKey(obj, fn);
 */
function eachKey (obj, fn, ctx) {
  fn = bind(fn, ctx);
  if (isObject(obj)) {
    var keys = Object.keys(obj);

    while (keys.length) {
      var key = keys.pop();
      fn(obj[key], key);
    }
  }
  return obj;
}

/**
 * map
 * -----
 *
 * Get a new array with values calculated from original array.
 *
 *     var arr = [1, 2, 3];
 *     var newArr = u.map(arr, fn);
 */
function map (arr, fn, ctx) {
  fn = bind(fn, ctx);
  var i = arr.length,
      mapped = new Array(i);
  while (--i > -1) {
    mapped[i] = fn(arr[i], i);
  }
  return mapped;
}

/**
 * clean
 * -----
 *
 * Clean an array by reference.
 *
 *     var arr = [1, 2, 3];
 *     u.clean(arr); // arr = []
 */
function clean (arr) {
  eachPop(arr, noop);
  return arr;
}

/**
 * range
 * -----
 *
 * Create an array of numbers from start to end, incremented by step.
 */
function range (start, end, step) {
  step = isNumber(step) ? step : 1;
  if (isUndefined(end)) {
    end = start;
    start = 0;
  }

  var i = Math.max(Math.ceil((end - start) / step), 0),
      result = new Array(i);

  while (--i > -1) {
    result[i] = start + (step * i);
  }
  return result;
}

/**
 * sortedIndex
 * -----------
 *
 * Finds the sorted position of a number in an Array of numbers.
 */
function sortedIndex (arr, n) {
  var min = 0,
      max = arr.length;

  while (min < max) {
    var mid = min + max >>> 1;
    if (n < arr[mid]) max = mid;
    else min = mid + 1;
  }

  return min;
}

/**
 * indexOf
 * -------
 *
 * Finds the index of a variable in an array.
 * Returns -1 if not found.
 */
function indexOf (arr, n) {
  var i = arr.length;
  while (--i > -1) {
    if (arr[i] === n) return i;
  }
  return i;
}

/**
 * uniqueInsert
 * ------------
 *
 * Inserts a value into an array only if it does not already exist
 * in the array.
 */
function uniqueInsert (arr, n) {
  if (indexOf(arr, n) === -1) arr.push(n);
  return arr;
}

/**
 * extend
 * ------
 *
 * Extend an object with the properties of one other objects
 */
function extend (obj, source) {
  if (isObject(obj) && isObject(source)) {
    var props = Object.getOwnPropertyNames(source),
      i = props.length;
    while (--i > -1) {
      var prop = props[i];
      obj[prop] = source[prop];
    }
  }
  return obj;
}

/**
   * bind
   * ----
   *
   * Bind a function to a context. Optionally pass in the number of arguments
   * which will use the faster fn.call if the number of arguments is 0, 1, or 2.
   */
function bind (fn, ctx) {
  if (!ctx) return fn;
  return function () { return fn.apply(ctx, arguments); };
}

/**
 * isUndefined
 * -----------
 *
 * Checks if a variable is undefined.
 */
function isUndefined (o) {
  return typeof o === 'undefined';
}

/**
 * isFunction
 * ----------
 *
 * Checks if a variable is a function.
 */
function isFunction (o) {
  return typeof o === 'function';
}

/**
 * isObject
 * --------
 *
 * Checks if a variable is an object.
 */
function isObject (o) {
  return typeof o === 'object' && !!o;
}

/**
 * isNumber
 * --------
 *
 * Checks if a variable is a number.
 */
function isNumber (o) {
  return typeof o === 'number';
}

/**
 * isNaN
 * -----
 *
 * Checks if a variable is NaN.
 */
function isNaN (o) {
  return isNumber(o) && o !== +o;
}

}, {}]}, {}, {"1":""})
);