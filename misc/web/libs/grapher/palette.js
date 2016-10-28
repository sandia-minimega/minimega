;(function () {
  var palette = function (g) {

    /**
      * grapher.palette
      * ------------------
      * 
      * Set a grapher to use a pre-defined palette. Palettes can be pre-defined
      * with the static function Grapher.setPalette.
      */
    g.prototype.palette = function (name) {
      if (g.utils.isUndefined(name)) return this.props.palette;

      this.props.palette = g.getPalette(name);
      this.update();
      return this;
    };

    /**
      * Extend _findColor to look for colors in palette
      */

    var _findColor = g.prototype._findColor;

    g.prototype._findColor = function (c) {
      var palette = this.palette();
      if (palette && palette[c]) color = palette[c];
      else color = _findColor.apply(this, arguments);

      return color;
    };

  /**
    * Grapher Static Properties
    * =========================
    */

    g.palettes = {}; // Store palettes staticly.

    /**
      * Grapher.getPalette
      * -------------------
      * 
      * Get a palette that has been defined.
      *
      */
    g.getPalette = function (name) { return this.palettes[name]; };

    /**
      * Grapher.setPalette
      * -------------------
      * 
      * Define a palette with a name and an array of color swatches.
      *
      */
    g.setPalette = function (name, swatches) {
      var palette = this.palettes[name] = {};
      swatches = g.utils.map(swatches, g.Color.parse);

      g.utils.each(swatches, function (swatch, i) {
        palette[i] = swatch;
        for (var j = 0; j < i; j++) { // Interpolate 'in-between' link colors 50% between node colors.
          var color = g.Color.interpolate(swatches[j], swatch, 0.5);
          palette[j + '-' + i] = color;
        }
      }, this);
      return this;
    };

  };

  if (typeof module !== 'undefined' && module.exports) module.exports = palette;
  else palette(Grapher);
})();
