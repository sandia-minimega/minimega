(function() {
  // MmModal is an "abstract" Vue component that contains a Bootstrap 3
  // modal. It provides "slots" where descendent components can add
  // content. To add content to the body of the modal, just add content
  // between the mm-modal tags.
  //
  // Component Properties:
  //
  //    - title: The title to display in the modal
  //
  // Events:
  //
  //   - closed: Fires whenever the modal is dismissed (Escape key,
  //             close button, etc.)
  //
  // Example:
  //
  //     <mm-modal
  //       :title="selectedVm.name"
  //       v-on:closed="doStuff()">
  //           <!-- The body just goes in here -->
  //           <p>Lorem ipsum...</p>
  //     </mm-modal>
  //

  const template = `
      <div class="modal fade" ref="modal">
          <div class="modal-dialog">
              <div class="modal-content">
                  <div class="modal-header">
                      <button
                          type="button"
                          class="close"
                          v-on:click="$emit('closed')">
                              <span>&times;</span>
                      </button>
                      <h4 class="modal-title">{{ title }}</h4>
                  </div>
                  <div class="modal-body">
                      <slot><i>Body not provided.</i></slot>
                  </div>
                  <div class="modal-footer">
                      <button
                          type="button"
                          class="btn btn-default"
                          v-on:click="$emit('closed')">
                              Close
                      </button>
                  </div>
              </div>
          </div>
      </div>
      `;

  window.MmModal = {
    template: template,

    // Component properties. This is the data that is passed to the
    // mm-modal tag.
    //
    // These values should be treated as read-only.
    props: {
      // The title shown on the modal
      title: {
        type: String,
      },
    },

    // Runs after the Vue component has been mounted and is
    // ready-to-go
    mounted() {
      // Show it immediately
      $(this.$refs['modal']).modal('show');

      // Emit 'closed' whenever the modal is closed
      $(this.$refs['modal']).on('hidden.bs.modal', () => {
        this.$emit('closed');
      });
    },

    // Runs right before the Vue component is cleaned up
    beforeDestroy() {
      // Before it's cleaned up, make sure the modal is hidden
      $(this.$refs['modal']).modal('hide');

      // ... and get rid of the event handler we set up
      $(this.$refs['modal']).off('hidden.bs.modal');
    },
  };
})();
