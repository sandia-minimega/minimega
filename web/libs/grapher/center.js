;(function () {
  var center = function (g) {

  /**
    * grapher.center
    * ------------------
    * 
    * Center the whole network or provided nodeIds in the view.
    */
    g.prototype.center = function (nodeIds) {
      var x = 0,
          y = 0,
          scale = 1,
          allNodes = this.data() ? this.data().nodes : null,
          nodes = [];
      if (nodeIds) for (i = 0; i < nodeIds.length; i++) { nodes.push(allNodes[nodeIds[i]]); }
      else nodes = allNodes;

      var numNodes = nodes ? nodes.length : 0;

      if (numNodes) { // get initial transform
        var minX = Infinity, maxX = -Infinity,
            minY = Infinity, maxY = -Infinity,
            width = this.width(),
            height = this.height(),
            pad = 1.1,
            i;

        for (i = 0; i < numNodes; i++) {
          if (nodes[i].x < minX) minX = nodes[i].x;
          if (nodes[i].x > maxX) maxX = nodes[i].x;
          if (nodes[i].y < minY) minY = nodes[i].y;
          if (nodes[i].y > maxY) maxY = nodes[i].y;
        }
        
        var dX = maxX - minX,
            dY = maxY - minY;

        scale = Math.min(width / dX, height / dY, 2) / pad;
        x = (width - dX * scale) / 2 - minX * scale;
        y = (height - dY * scale) / 2 - minY * scale;
      }

      return this.scale(scale).translate([x, y]);
    };

  /**
    * grapher.centerToPoint
    * ------------------
    * 
    * Center the network to the point with x and y coordinates
    */
    g.prototype.centerToPoint = function (point) {
      var width = this.width(),
          height = this.height(),
          x = this.translate()[0] + width / 2 - point.x,
          y = this.translate()[1] + height / 2 - point.y;

      return this.translate([x, y]);
    };

  /**
    * Extend data to call this.center,
    * scale and translate to track when the user modifies the transform.
    */
    var render = g.prototype.render,
        scale = g.prototype.scale,
        translate = g.prototype.translate;

    g.prototype._hasModifiedTransform = false;
    g.prototype.render = function () {
      if (!this._hasModifiedTransform) this.center();
      return render.apply(this, arguments);
    };
    g.prototype.scale = function () {
      var res = scale.apply(this, arguments);
      if (res === this) this._hasModifiedTransform = true;
      return res;
    };
    g.prototype.translate = function () {
      var res = translate.apply(this, arguments);
      if (res === this) this._hasModifiedTransform = true;
      return res;
    };
  };

  if (typeof module !== 'undefined' && module.exports) module.exports = center;
  else center(Grapher);
})();
