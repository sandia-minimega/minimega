(function() {
  const template = `
      <div class="modal fade mdl" tabindex="-1" ref="lmodal">
        <div class="modal-dialog modal-dialog-centered mdl" role="document">
          <div class="modal-content mdl">
            <div class="modal-header m-3 mdl">
              <h5 class="modal-title text-center col-12 mdl"><b class="mdl">{{ header }}</b></h5>
            </div>
            <div class="modal-body m-3 mdl">
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
        type: String
      },
      body: {
        type: String
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
