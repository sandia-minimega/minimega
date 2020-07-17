/*
 * EditReservationModal.js
 *
 * The EditReservationModal component allows a user to edit certain
 * aspects of a reservation.
 *
 * Initially, the modal is hidden. To show the modal, the properties
 * of the EditReservationModal component should be set, then the
 * "show()" method can be called. The modal will hide itself
 * automatically when the user submits a command or closes it
 * manually. If necessary, the "hide()" method also closes it.
 *
 */
(function() {
  const template = `
    <div>
      <!-- Edit reservation modal -->
      <div
        aria-hidden="true"
        aria-labelledby="Edit Reservation"
        class="modal fade"
        ref="modal"
        role="dialog"
        tabindex="-1"
      >
        <div class="modal-dialog modal-dialog-centered" role="document">
          <div class="modal-content">
            <div class="modal-header m-3">
              <h5 class="modal-title text-center col-12">
                <b>Edit Reservation</b>
              </h5>
              <button
                aria-label="Close"
                class="close"
                data-dismiss="modal"
                style="position: absolute; right: 15px; top: 10px;"
                type="button"
              >
                <span aria-hidden="true">&times;</span>
              </button>
            </div>
            <div class="modal-body m-3">
              <form>
                <!-- Reservation name, -r -->
                <div class="form-group">
                  <div
                    class="input-group"
                    data-placement="bottom"
                    data-toggle="tooltip"
                    title="Reservation name"
                  >
                    <div class="input-group-prepend">
                      <div class="input-group-text">
                        <code>-r</code>
                      </div>
                    </div>
                    <input
                      autofocus
                      class="dash form-control"
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
                  class="btn-group"
                  role="group"
                  style="width: 100%; margin-bottom: 0;"
                >
                  <button
                    :class="{active: isKernelInit}"
                    class="modalswitch btn btn-light"
                    style="width: 50%;"
                    type="button"
                    v-on:click="isKernelInit = true"
                  >Use kernel and initrd</button>
                  <button
                    :class="{active: !isKernelInit}"
                    class="modalswitch btn btn-light"
                    style="width: 50%;"
                    type="button"
                    v-on:click="isKernelInit = false"
                  >Use Cobbler profile</button>
                </div>
                <!-- Kernel path, -k, only shows if left side of above switch is active -->
                <div
                  class="form-group"
                  style="margin-bottom: 10px;"
                  v-if="isKernelInit"
                >
                  <div
                    class="input-group"
                    data-placement="bottom"
                    data-toggle="tooltip"
                    title="Location of the kernel the nodes should boot. This kernel will be copied to a separate directory for use."
                  >
                    <div class="input-group-prepend">
                      <div class="input-group-text">
                        <code>-k</code>
                      </div>
                    </div>
                    <input
                      :class="{'is-valid': kernelPath && kernelPathIsValid, 'is-invalid': !kernelPathIsValid}"
                      class="dash form-control"
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
                <div class="form-group" v-if="isKernelInit">
                  <div
                    class="input-group"
                    data-placement="bottom"
                    data-toggle="tooltip"
                    title="Location of the initrd the nodes should boot. This file will be copied to a separate directory for use."
                  >
                    <div class="input-group-prepend">
                      <div class="input-group-text">
                        <code>-i</code>
                      </div>
                    </div>
                    <input
                      :class="{'is-valid': initrdPath && initrdPathIsValid, 'is-invalid': !initrdPathIsValid}"
                      class="dash form-control"
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
                  <div>
                    <select v-model="kernelPair">
                      <option disabled value>Choose a kernel pair</option>
                      <option
                        v-bind:value="{kernelPath: item.kernel, initrdPath: item.initrd}"
                        v-for="item in images"
                      >{{ item.name }}</option>
                    </select>
                  </div>
                </div>
                <!-- Cobbler profile, -profile, only shows if right side of above switch is active -->
                <div class="form-group" v-if="!isKernelInit">
                  <div class="input-group">
                    <div class="input-group-prepend">
                      <div class="input-group-text">
                        <code>-profile</code>
                      </div>
                    </div>
                    <input
                      class="dash form-control"
                      placeholder="Cobbler profile"
                      type="text"
                      v-model="cobblerProfile"
                    >
                  </div>
                </div>

                <!-- The rest of the fields are optional -->
                <i class="mb-2">Optional:</i>
                <div
                  class="mb-4"
                  style="border-top: 1px solid #e9ecef; border-bottom: 1px solid #e9ecef; padding-top: 5px;"
                >
                  <!-- Group, -g, optional -->
                  <div class="form-group">
                    <div
                      class="input-group"
                      data-placement="bottom"
                      data-toggle="tooltip"
                      title="An optional group that can modify this reservation"
                    >
                      <div class="input-group-prepend">
                        <div class="input-group-text">
                          <code style="color: royalblue;">-g</code>
                        </div>
                      </div>
                      <input
                        :class="{'is-valid': group && groupIsValid, 'is-invalid': group && !groupIsValid}"
                        class="dash form-control"
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
                  <div class="form-group">
                    <div
                      class="input-group"
                      data-placement="bottom"
                      data-toggle="tooltip"
                      title="e.g. console=tty0"
                    >
                      <div class="input-group-prepend">
                        <div class="input-group-text">
                          <code style="color: royalblue;">-c</code>
                        </div>
                      </div>
                      <input
                        class="dash form-control"
                        placeholder="Command line arguments"
                        type="text"
                        v-model="cmdArgs"
                      >
                    </div>
                  </div>
                  <!-- VLAN, -vlan, optional -->
                  <div class="form-group">
                    <div
                      class="input-group"
                      data-placement="bottom"
                      data-toggle="tooltip"
                      title="Specifies a VLAN to use. May be a VLAN ID or the name of an existing reservation. If a reservation name is provided, the reservation will be modified, so that it is on the same VLAN as the specified reservation."
                    >
                      <div class="input-group-prepend">
                        <div class="input-group-text">
                          <code style="color: royalblue;">-vlan</code>
                        </div>
                      </div>
                      <input
                        :class="{'is-valid': vlan && vlanIsValid, 'is-invalid': vlan && !vlanIsValid}"
                        class="dash form-control"
                        placeholder="VLAN"
                        type="text"
                        v-model="vlan"
                      >
                      <div
                        class="valid-feedback"
                        v-if="vlan && vlanIsValid"
                      >Looking good!</div>
                      <div
                        class="invalid-feedback"
                        v-if="vlan && !vlanIsValid"
                      >Invalid value for VLAN</div>
                    </div>
                  </div>
                </div>
              </form>

              <div class="card commandline">
                <code id="commandline" style="color: seagreen;">{{ command }}</code>
              </div>
            </div>
            <div class="modal-footer m-3">
              <button
                class="modalbtn igorbtn btn btn-secondary mr-auto cancel"
                data-dismiss="modal"
                type="button"
              >Cancel</button>

              <button
                :disabled="!validForm"
                class="modalbtn igorbtn btn btn-primary modalcommand"
                style="background-color: #a975d6; border-color: #a975d6;"
                type="button"
                v-on:click="submitUpdate()"
              >
                <span>Update Reservation</span>
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
        kernelPair: '',
        kernelPath: '',
        initrdPath: '',
        cobblerProfile: '',
        group: '',
        cmdArgs: '',
        vlan: '',

        isKernelInit: true,

        serverMessage: '',
        serverSuccess: true,
      };
    },

    watch: {
      kernelPath() {
        this.kernelPair = '';
      },
      initrdPath() {
        this.kernelPair = '';
      },
      kernelPair(value) {
        if (value) {
          this.kernelPath = value.kernelPath;
          this.initrdPath = value.initrdPath;
        }
      },
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

      vlanIsValid() {
        if (!isNaN(Number(this.vlan))) {
          return true;
        }

        return this.$store.getters.reservations.some(r => r.Name == this.vlan);
      },

      images() {
        return this.$store.getters.allImages;
      },

      validForm() {
        if (!this.name) {
          return false;
        }

        if (this.group && !this.groupIsValid) {
          return false;
        }

        if (this.vlan && !this.vlanIsValid) {
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

        // Check that *something* has been edited.
        if ([this.kernelPath, this.initrdPath, this.cobblerProfile, this.group, this.cmdArgs, this.vlan].every((x) => x == '')) {
          return false;
        }

        return true;
      },

      command() {
        let bootFrom = '';
        if (this.isKernelInit && (this.kernelPath != '' && this.initrdPath != '')) {
          bootFrom = ` -k ${this.kernelPath} -i ${this.initrdPath}`;
        } else if (this.cobblerProfile != '') {
          ` -profile ${this.cobblerProfile}`;
        }

        let group = '';
        if (this.group) {
          group = ` -g ${this.group}`;
        }

        let vlan = '';
        if (this.vlan) {
          group = ` -vlan ${this.vlan}`;
        }

        let args = '';
        if (this.cmdArgs) {
          args = ` -c ${this.cmdArgs}`;
        }

        return `igor edit -r ${this.name}${bootFrom}${args}${group}${vlan}`;
      },
    },

    methods: {
      show(resName) {
        // Find the matching reservation
        let res = null;
        for (let i = 0; i < this.$store.state.reservations.length; i++) {
          const r = this.$store.state.reservations[i];
          if (r['Name'] == resName) {
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
