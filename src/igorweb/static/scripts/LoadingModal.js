(function() {
  const template = `
    <div class="modal fade" ref="lmodal" tabindex="-1">
      <div class="modal-dialog modal-dialog-centered" role="document">
        <div class="modal-content">
          <div class="modal-header m-3">
            <h5 class="modal-title text-center col-12">
              <b>{{ header }}</b>
            </h5>
          </div>
          <div class="modal-body m-3">
            <p>{{ body }}</p>
          </div>
        </div>
      </div>
    </div>
  `;

  window.LoadingModal = {
    template: template,

    props: {
      header: {
        type: String,
      },
      body: {
        type: String,
      },
    },

    methods: {
      show() {
        // Don't allow the user to dismiss this modal
        $(this.$refs['lmodal']).modal({backdrop: 'static', keyboard: false});
      },

      hide() {
        $(this.$refs['lmodal']).modal('hide');
      },
    },
  };
})();
