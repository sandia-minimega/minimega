const template = `
    <div>
      <div class="well well-sm">
        <canvas ref="my-canvas"></canvas>
        <slot></slot>
      </div>
    </div>
    `;

export var MmCanvas = {
    template: template,

    data() {
        return {
            provider: {
                context: null
            }
        }
    },

    provide () {
        return {
            provider: this.provider
        };
    },

    mounted () {
        this.provider.context = this.$refs['my-canvas'].getContext('2d');

        this.$refs['my-canvas'].width = this.$refs['my-canvas'].parentElement.clientWidth;
        this.$refs['my-canvas'].height = this.$refs['my-canvas'].parentElement.clientHeight;
    }
};
