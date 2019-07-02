(function() {
  const template = `
    <div id="outer">
      <!-- Edit reservation modal -->
      <div
        aria-hidden="true"
        aria-labelledby="Edit Reservation"
        class="modal fade mdl"
        ref="modal"
        role="dialog"
        tabindex="-1"
      >
        <div class="modal-dialog modal-dialog-centered mdl" role="document">
          <div class="modal-content mdl">
            <div class="modal-header m-3 mdl">
              <h5 class="modal-title text-center col-12 mdl" id="modaltitle">
                <b class="mdl">Edit Reservation</b>
              </h5>
              <button
                aria-label="Close"
                class="close mdl"
                data-dismiss="modal"
                style="position: absolute; right: 15px; top: 10px;"
                type="button"
              >
                <span aria-hidden="true" class="mdl">&times;</span>
              </button>
            </div>
            <div class="modal-body m-3 mdl">
              <form class="mdl">
                <!-- Reservation name, -r -->
                <div class="form-group mdl">
                  <div
                    class="input-group mdl"
                    data-placement="bottom"
                    data-toggle="tooltip"
                    title="Reservation name"
                  >
                    <div class="input-group-prepend mdl">
                      <div class="input-group-text mdl">
                        <code class="mdl" id="dashrcode">-r</code>
                      </div>
                    </div>
                    <input
                      autofocus
                      class="dash form-control mdl"
                      disabled
                      placeholder="Reservation name"
                      type="text"
                      v-model="name"
                    >
                  </div>
                </div>
                <!-- Switch for (kernel and initrd) or (cobbler profile) -->
                <div
                  aria-label="Use kernel and initrd or Cobbler profile?"
                  class="mdl btn-group"
                  role="group"
                  style="width: 100%; margin-bottom: 0;"
                >
                  <button
                    :class="{active: isKernelInit}"
                    class="modalswitch btn btn-light mdl"
                    style="width: 50%;"
                    type="button"
                    v-on:click="isKernelInit = true"
                  >Use kernel and initrd</button>
                  <button
                    :class="{active: !isKernelInit}"
                    class="modalswitch btn btn-light mdl"
                    style="width: 50%;"
                    type="button"
                    v-on:click="isKernelInit = false"
                  >Use Cobbler profile</button>
                </div>
                <!-- Kernel path, -k, only shows if left side of above switch is active -->
                <div
                  class="form-group switchki mdl"
                  style="margin-bottom: 10px;"
                  v-if="isKernelInit"
                >
                  <div
                    class="input-group mdl"
                    data-placement="bottom"
                    data-toggle="tooltip"
                    id="dashkparent"
                    title="Location of the kernel the nodes should boot. This kernel will be copied to a separate directory for use."
                  >
                    <div class="input-group-prepend mdl">
                      <div class="input-group-text mdl">
                        <code class="mdl" id="dashkcode">-k</code>
                      </div>
                    </div>
                    <input
                      :class="{'is-valid': kernelPath && kernelPathIsValid, 'is-invalid': !kernelPathIsValid}"
                      class="dash form-control mdl"
                      placeholder="Kernel path"
                      type="text"
                      v-model="kernelPath"
                    >
                    <div
                      class="valid-feedback"
                      v-if="kernelPathIsValid"
                    >Looking good!</div>
                    <div
                      class="invalid-feedback"
                      v-if="!kernelPathIsValid"
                    >Path must be an absolute path to a kernel.</div>
                  </div>
                </div>
                <!-- Initrd path, -i, only shows if left side of above switch is active -->
                <div class="form-group switchki mdl" v-if="isKernelInit">
                  <div
                    class="input-group mdl"
                    data-placement="bottom"
                    data-toggle="tooltip"
                    id="dashiparent"
                    title="Location of the initrd the nodes should boot. This file will be copied to a separate directory for use."
                  >
                    <div class="input-group-prepend mdl">
                      <div class="input-group-text mdl">
                        <code class="mdl" id="dashicode">-i</code>
                      </div>
                    </div>
                    <input
                      :class="{'is-valid': initrdPath && initrdPathIsValid, 'is-invalid': !initrdPathIsValid}"
                      class="dash form-control mdl"
                      placeholder="Initrd path"
                      type="text"
                      v-model="initrdPath"
                    >

                    <div
                      class="valid-feedback"
                      v-if="initrdPathIsValid"
                    >Looking good!</div>
                    <div
                      class="invalid-feedback"
                      v-if="!initrdPathIsValid"
                    >Path must be an absolute path to an initial RAM disk.</div>
                  </div>
                </div>
                <!-- Cobbler profile, -profile, only shows if right side of above switch is active -->
                <div class="form-group switchcobbler mdl" v-if="!isKernelInit">
                  <div class="input-group mdl" id="dashpparent">
                    <div class="input-group-prepend mdl">
                      <div class="input-group-text mdl">
                        <code class="mdl" id="dashpcode">-profile</code>
                      </div>
                    </div>
                    <input
                      class="dash form-control mdl"
                      placeholder="Cobbler profile"
                      type="text"
                      v-model="cobblerProfile"
                    >
                  </div>
                </div>

                <!-- The rest of the fields are optional -->
                <i class="mb-2 mdl">Optional:</i>
                <div
                  class="mb-4 mdl"
                  style="border-top: 1px solid #e9ecef; border-bottom: 1px solid #e9ecef; padding-top: 5px;"
                >
                  <!-- Group, -g, optional -->
                  <div class="form-group mdl">
                    <div
                      class="input-group mdl"
                      data-placement="bottom"
                      data-toggle="tooltip"
                      title="An optional group that can modify this reservation"
                    >
                      <div class="input-group-prepend mdl">
                        <div class="input-group-text mdl">
                          <code
                            class="mdl"
                            id="dashccode"
                            style="color: royalblue;"
                          >-g</code>
                        </div>
                      </div>
                      <input
                        :class="{'is-valid': group && groupIsValid, 'is-invalid': group && !groupIsValid}"
                        class="dash form-control mdl"
                        placeholder="Group"
                        type="text"
                        v-model="group"
                      >
                      <div
                        class="valid-feedback"
                        v-if="group && groupIsValid"
                      >Looking good!</div>
                      <div
                        class="invalid-feedback"
                        v-if="group && !groupIsValid"
                      >Invalid group name.</div>
                    </div>
                  </div>
                  <!-- Command line arguments, -c, optional -->
                  <div class="form-group mdl">
                    <div
                      class="input-group mdl"
                      data-placement="bottom"
                      data-toggle="tooltip"
                      title="e.g. console=tty0"
                    >
                      <div class="input-group-prepend mdl">
                        <div class="input-group-text mdl">
                          <code
                            class="mdl"
                            id="dashccode"
                            style="color: royalblue;"
                          >-c</code>
                        </div>
                      </div>
                      <input
                        class="dash form-control mdl"
                        placeholder="Command line arguments"
                        type="text"
                        v-model="cmdArgs"
                      >
                    </div>
                  </div>
                </div>
              </form>

              <div class="card commandline mdl">
                <code
                  class="mdl"
                  id="commandline"
                  style="color: seagreen;"
                >{{ command }}</code>
              </div>
            </div>
            <div class="modal-footer m-3 mdl">
              <button
                class="modalbtn igorbtn btn btn-secondary mr-auto mdl cancel"
                data-dismiss="modal"
                type="button"
              >Cancel</button>

              <button
                :disabled="!validForm"
                class="modalbtn newresmodalgobtn igorbtn btn btn-primary mdl modalcommand"
                style="background-color: #a975d6; border-color: #a975d6;"
                type="button"
                v-on:click="submitUpdate()"
              >
                <span class="mdl mdlcmdtext">Update Reservation</span>
              </button>
            </div>
          </div>
        </div>
      </div>

      <loading-modal
        body="This may take some time..."
        header="Submitting reservation"
        ref="loadingModal"
      ></loading-modal>
    </div>
  `;

  window.EditReservationModal = {
    template: template,

    components: {
      LoadingModal,
    },

    data() {
      return {
        name: '',
        kernelPath: '',
        initrdPath: '',
        cobblerProfile: '',
        group: '',
        cmdArgs: '',

        isKernelInit: true,

        serverMessage: '',
        serverSuccess: true,
      };
    },

    computed: {
      groupIsValid() {
        const re = new RegExp('^[_a-z][0-9a-z_-]*\\$?$');
        return this.group.match(re) != null;
      },

      kernelPathIsValid() {
        const re = new RegExp('^(/[^/]*)+[^/]+\\.kernel$');
        return this.kernelPath.match(re) != null || this.kernelPath == '';
      },

      initrdPathIsValid() {
        const re = new RegExp('^(/[^/]*)+[^/]+\\.initrd$');
        return this.initrdPath.match(re) != null || this.initrdPath == '';
      },

      validForm() {
        if (!this.name) {
          return false;
        }

        if (this.group && !this.groupIsValid) {
          return false;
        }

        if (this.isKernelInit) {
          if (!this.kernelPathIsValid || !this.initrdPathIsValid) {
            return false;
          }
        } else {
          if (!this.cobblerProfile) {
            return false;
          }
        }

        if ([this.kernelPath, this.initrdPath, this.cobblerProfile, this.group, this.cmdArgs].every((x) => x == "")) {
          return false;
        }

        return true;
      },

      command() {
        let bootFrom = '';
        if (this.isKernelInit && (this.kernelPath != "" + this.initrdPath != "")) {
          bootFrom = ` -k ${this.kernelPath} -i ${this.initrdPath}`;
        } else if (this.cobblerProfile != "") {
          ` -profile ${this.cobblerProfile}`;
        }

        let group = '';
        if (this.group) {
          group = ` -g ${this.group}`;
        }

        let args = '';
        if (this.cmdArgs) {
          args = ` -c ${this.cmdArgs}`;
        }

        return `igor edit -r ${this.name}${bootFrom}${args}${group}`;
      },
    },

    methods: {
      show(resName) {
        // Find the matching reservation
        let res = null;
        for (let i = 0; i < this.$store.state.reservations.length; i++) {
          let r = this.$store.state.reservations[i];
          if (r["Name"] == resName) {
            res = r;
          }
        }

        if (res != null) {
          this.name = res.Name;
          this.kernelPath = res.Kernel;
          this.initrdPath = res.Initrd;
          this.cobblerProfile = res.CobblerProfile;
          this.cmdArgs = res.KernelArgs;
        }

        $(this.$refs['modal']).modal('show');
      },

      hide() {
        $(this.$refs['modal']).modal('hide');
      },

      showLoading() {
        this.$refs['loadingModal'].show();
      },

      hideLoading() {
        setTimeout(this.$refs['loadingModal'].hide, 500);
      },

      submitUpdate() {
        if (this.validForm) {
          this.showLoading();
          this.hide();

          $.get(
              'run/',
              {run: this.command},
              (data) => {
                const response = JSON.parse(data);
                this.$store.commit('updateReservations', response.Extra);
                this.$store.commit('setAlert', `Reservation ${this.name}: ${response.Message}`);
                this.hideLoading();
              }
          );
        }
      },
    },
  };
})();
