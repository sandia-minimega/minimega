(function() {
  const template = `
      <div>
        <div class="well well-sm">
          <canvas ref="my-canvas"></canvas>
          <slot></slot>
        </div>
      </div>
      `;

  // MmCanvas is an "abstract" Vue component that contains a canvas
  // element. It provides an object named "provider", which allows
  // descendents to manipulate the provided canvas.
  window.MmCanvas = {

    // HTML template for this component
    template: template,

    // Local data for this component
    data() {
      return {
        // The provider with its canvas context
        provider: {
          context: null,
        },
      };
    },

    // Provide provider to descendents
    provide() {
      return {
        provider: this.provider,
      };
    },

    // Runs after this Vue component has been mounted and is
    // ready-to-go
    mounted() {
      // Grab the canvas context
      this.provider.context = this.$refs['my-canvas'].getContext('2d');

      // Set the size of the canvas according to the size of its parent element
      this.$refs['my-canvas'].width =
        this.$refs['my-canvas'].parentElement.clientWidth;

      this.$refs['my-canvas'].height =
        this.$refs['my-canvas'].parentElement.clientHeight;
    },
  };
})();
