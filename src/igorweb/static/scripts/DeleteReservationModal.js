(function() {
  const template = `
    <div>
      <!-- Delete reservation modal -->
      <div class="modal fade mdl" id="deleteresmodal" tabindex="-1" role="dialog" aria-labelledby="Delete Reservation?" aria-hidden="true" ref="modal">
        <div id="deletemodaldialog" class="modal-dialog modal-sm modal-dialog-centered mdl" role="document">
          <div class="modal-content mdl">
            <div class="modal-header m-3 mdl" style="padding-bottom: 0px; margin-bottom: 5px !important; border: none;">
              <h5 class="modal-title text-center col-12 mdl" id="dmodaltitle"><b class="mdl">Delete reservation "{{reservation.Name}}"?</b></h5>
              <button type="button" class="close mdl" data-dismiss="modal" aria-label="Close" style="position: absolute; right: 15px; top: 10px;">
                <span class="mdl" aria-hidden="true">&times;</span>
              </button>
            </div>

            <!-- Buttons at bottom of modal -->
            <div class="modal-footer m-3 mdl" style="padding-top: 20px; margin-top: 20px;">
              <!-- Cancel, exits modal, only shows on main reservation page -->
              <button type="button" class="modalbtn igorbtn btn btn-secondary mr-auto mdl cancel" data-dismiss="modal">Cancel</button>
              <!-- Delete, sends a igor del command to the server -->
              <button type="button" style="background-color: #a975d6; border-color: #a975d6;" class="modalbtn deleteresmodalgobtn igorbtn btn btn-primary mdl modalcommand" v-on:click="deleteReservation()"><span class="mdl mdlcmdtext">Delete</span></button>
            </div>
          </div>
        </div>
      </div>

      <loading-modal
        ref="loadingModal"
        header="Deleting Reservation"
        body="This may take some time..."
      >
      </loading-modal>
    </div>
    `;

  window.DeleteReservationModal = {
    template: template,

    components: {
      LoadingModal,
    },

    data() {
      return {
        reservation: {},
      };
    },

    methods: {
      show() {
        this.reservation = this.$store.state.selectedReservation;
        $(this.$refs['modal']).modal('show');
      },

      hide() {
        $(this.$refs['modal']).modal('hide');
      },

      showLoading() {
        this.$refs['loadingModal'].show();
      },

      hideLoading() {
        this.$refs['loadingModal'].hide();
      },

      deleteReservation() {
        this.hide();
        this.showLoading();

        $.get(
            'run/',
            {run: `igor del ${this.reservation.Name}`},
            (data) => {
              const response = JSON.parse(data);

              let msg = response.Message;
              if (msg == '\n') {
                msg = `Successfully deleted ${this.reservation.Name}`;
              }

              this.$store.commit('updateReservations', response.Extra);
              this.$store.commit('setAlert', msg);
              setTimeout(() => {
                this.hideLoading();
                this.$emit('deleted');
              }, 500);
            }
        );
      },
    },
  };
})();
