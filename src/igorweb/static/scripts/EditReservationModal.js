(function() {
  const template = `
      <div id="outer">
      <!-- Edit reservation modal -->
      <div class="modal fade mdl" tabindex="-1" role="dialog" aria-labelledby="Edit Reservation" aria-hidden="true" ref="modal">
        <div class="modal-dialog modal-dialog-centered mdl" role="document">
          <div class="modal-content mdl">
            <div class="modal-header m-3 mdl">
              <h5 class="modal-title text-center col-12 mdl" id="modaltitle">
                <b class="mdl">Edit Reservation</b>
              </h5>
              <button type="button" class="close mdl" data-dismiss="modal" aria-label="Close" style="position: absolute; right: 15px; top: 10px;">
                <span class="mdl" aria-hidden="true">&times;</span>
              </button>
            </div>
            <div class="modal-body m-3 mdl">
              <form class="mdl">
                <!-- Reservation name, -r -->
                <div class="form-group mdl">
                  <div class="input-group mdl" data-toggle="tooltip" data-placement="bottom" title="Reservation name">
                    <div class="input-group-prepend mdl">
                      <div class="input-group-text mdl"><code id="dashrcode" class="mdl">-r</code></div>
                    </div>
                    <input v-model="name" type="text" class="dash form-control mdl" placeholder="Reservation name" autofocus disabled>
                  </div>
                </div>
                <!-- Switch for (kernel and initrd) or (cobbler profile) -->
                <div class="mdl btn-group" role="group" aria-label="Use kernel and initrd or Cobbler profile?" style="width: 100%; margin-bottom: 0;">
                  <button type="button" class="modalswitch btn btn-light mdl" :class="{active: isKernelInit}" style="width: 50%;" v-on:click="isKernelInit = true">Use kernel and initrd</button>
                  <button type="button" class="modalswitch btn btn-light mdl" :class="{active: !isKernelInit}" style="width: 50%;" v-on:click="isKernelInit = false">Use Cobbler profile</button>
                </div>
                <!-- Kernel path, -k, only shows if left side of above switch is active -->
                <div v-if="isKernelInit" class="form-group switchki mdl" style="margin-bottom: 10px;">
                  <div id="dashkparent" class="input-group mdl" data-toggle="tooltip" data-placement="bottom" title="Location of the kernel the nodes should boot. This kernel will be copied to a separate directory for use.">
                    <div class="input-group-prepend mdl">
                      <div class="input-group-text mdl">
                        <code id="dashkcode" class="mdl">-k</code>
                      </div>
                    </div>
                    <input v-model="kernelPath" type="text" class="dash form-control mdl" :class="{'is-valid': kernelPath && kernelPathIsValid, 'is-invalid': !kernelPathIsValid}" placeholder="Kernel path">
                    <div v-if="kernelPathIsValid" class="valid-feedback">
                      Looking good!
                    </div>
                    <div v-if="!kernelPathIsValid" class="invalid-feedback">
                      Path must be an absolute path to a kernel.
                    </div>
                  </div>
                </div>
                <!-- Initrd path, -i, only shows if left side of above switch is active -->
                <div v-if="isKernelInit" class="form-group switchki mdl">
                  <div id="dashiparent" class="input-group mdl" data-toggle="tooltip" data-placement="bottom" title="Location of the initrd the nodes should boot. This file will be copied to a separate directory for use.">
                    <div class="input-group-prepend mdl">
                      <div class="input-group-text mdl"><code id="dashicode" class="mdl">-i</code></div>
                    </div>
                    <input v-model="initrdPath" type="text" class="dash form-control mdl" :class="{'is-valid': initrdPath && initrdPathIsValid, 'is-invalid': !initrdPathIsValid}" placeholder="Initrd path">

                    <div v-if="initrdPathIsValid" class="valid-feedback">
                      Looking good!
                    </div>
                    <div v-if="!initrdPathIsValid" class="invalid-feedback">
                      Path must be an absolute path to an initial RAM disk.
                    </div>
                  </div>
                </div>
                <!-- Cobbler profile, -profile, only shows if right side of above switch is active -->
                <div v-if="!isKernelInit" class="form-group switchcobbler mdl">
                  <div id="dashpparent" class="input-group mdl">
                    <div class="input-group-prepend mdl">
                      <div class="input-group-text mdl"><code id="dashpcode" class="mdl">-profile</code></div>
                    </div>
                    <input v-model="cobblerProfile" type="text" class="dash form-control mdl" placeholder="Cobbler profile">
                  </div>
                </div>

                <!-- The rest of the fields are optional -->
                <i class="mb-2 mdl">Optional:</i>
                <div class="mb-4 mdl" style="border-top: 1px solid #e9ecef; border-bottom: 1px solid #e9ecef; padding-top: 5px;">
                  <!-- Group, -g, optional -->
                  <div class="form-group mdl">
                    <div class="input-group mdl" data-toggle="tooltip" data-placement="bottom" title="An optional group that can modify this reservation">
                      <div class="input-group-prepend mdl">
                        <div class="input-group-text mdl"><code id="dashccode" class="mdl" style="color: royalblue;">-g</code></div>
                      </div>
                      <input v-model="group" type="text" class="dash form-control mdl" :class="{'is-valid': group && groupIsValid, 'is-invalid': group && !groupIsValid}" placeholder="Group">
                    <div v-if="group && groupIsValid" class="valid-feedback">
                      Looking good!
                    </div>
                    <div v-if="group && !groupIsValid" class="invalid-feedback">
                      Invalid group name.
                    </div>
                    </div>
                  </div>
                  <!-- Command line arguments, -c, optional -->
                  <div class="form-group mdl">
                    <div class="input-group mdl" data-toggle="tooltip" data-placement="bottom" title="e.g. console=tty0">
                      <div class="input-group-prepend mdl">
                        <div class="input-group-text mdl"><code id="dashccode" class="mdl" style="color: royalblue;">-c</code></div>
                      </div>
                      <input v-model="cmdArgs" type="text" class="dash form-control mdl" placeholder="Command line arguments">
                    </div>
                  </div>
                </div>
              </form>

              <div class="card commandline mdl">
                <code id="commandline" class="mdl" style="color: seagreen;">
                  {{ command }}
                </code>
              </div>
            </div>
            <div class="modal-footer m-3 mdl">
              <button
                type="button"
                class="modalbtn igorbtn btn btn-secondary mr-auto mdl cancel"
                data-dismiss="modal">
                  Cancel
              </button>

              <button
                type="button"
                style="background-color: #a975d6; border-color: #a975d6;"
                class="modalbtn newresmodalgobtn igorbtn btn btn-primary mdl modalcommand"
                :disabled="!validForm"
                v-on:click="submitUpdate()">
                  <span class="mdl mdlcmdtext">Update Reservation</span>
              </button>
            </div>
          </div>
        </div>
      </div>

        <loading-modal
          ref="loadingModal"
          header="Submitting reservation"
          body="This may take some time..."
        >
        </loading-modal>
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
