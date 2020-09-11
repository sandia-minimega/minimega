;(function () {
  var zoom = function (g) {

  /**
    * grapher.zoom
    * ------------------
    * 
    * Zoom the graph by some ratio. Optionally pass in x and y zoom center.
    */
    g.prototype.zoom = function (ratio, options) {
      if (ratio) {
        var transform = this.transform(),
            width = this.width(),
            height = this.height();

        var o = {
              x: width / 2,
              y: height / 2
            };

        if (options) {
          if (typeof options.x === 'number') o.x = options.x;
          if (typeof options.y === 'number') o.y = options.y;
        }

        var scale = transform.scale * ratio,
            midX = o.x - transform.translate[0],
            midY = o.y - transform.translate[1],
            x = transform.translate[0] - (midX * ratio - midX),
            y = transform.translate[1] - (midY * ratio - midY);

        this.scale(scale).translate([x, y]);
      }
      return this;
    };

  };

  if (typeof module !== 'undefined' && module.exports) module.exports = zoom;
  else zoom(Grapher);
})();
