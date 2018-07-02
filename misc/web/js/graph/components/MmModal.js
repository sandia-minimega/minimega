const template = `
    <div class="modal fade" ref="modal">
        <div class="modal-dialog">
            <div class="modal-content">
                <div class="modal-header">
                    <button type="button" class="close" v-on:click="$emit('closed')">
                        <span>&times;</span>
                    </button>
                    <h4 class="modal-title">{{ title }}</h4>
                </div>
                <div class="modal-body">
                    <slot><i>Body not provided.</i></slot>
                </div>
                <div class="modal-footer">
                    <button type="button" class="btn btn-default" v-on:click="$emit('closed')">Close</button>
                </div>
            </div>
        </div>
    </div>
    `;

export var MmModal = {
    template: template,

    props: {
        title: {
            type: String,
        },
    },

    mounted() {
        $(this.$refs['modal']).modal('show');
        $(this.$refs['modal']).on('hidden.bs.modal', () => {
            this.$emit('closed');
        });
    },

    beforeDestroy() {
        $(this.$refs['modal']).modal('hide');
        $(this.$refs['modal']).off('hidden.bs.modal');
    },
};
