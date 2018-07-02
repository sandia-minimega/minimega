import {formatVMField} from '../filters/formatVMField.js';
import {VmDetails} from './VmDetails.js';
import {MmModal} from './MmModal.js';


const template = `
    <div>
        <div class="btn-group pull-right">
            <button type="button"
                    class="btn btn-default"
                    v-bind:class="{ active: isVisible(field) }"
                    v-for="field in fields"
                    v-on:click="press(field)">
                {{ field | formatVMField }}
            </button>
        </div>

        <h3 v-if="title">{{ title }}</h3>

        <table class="table table-striped table-bordered">
            <thead>
                <tr>
                    <th v-for="field in visible"
                        v-on:click="toggleSort(field)">
                        {{ field | formatVMField }}
                        <span v-if="field === sortKey">
                            <i v-if="reverse" class="fa fa-arrow-up"></i>
                            <i v-if="!reverse" class="fa fa-arrow-down"></i>
                        </span>
                    </th>
                    <th v-if="!disableDetails"
                        style="width:0;">
                        Info
                    </th>
                </tr>
            </thead>
            <tbody>
                <tr v-for="vm in vms">
                    <td v-for="field in visible">
                        {{ vm[field] }}
                    </td>
                    <td v-if="!disableDetails">
                        <button
                          class="btn btn-info"
                          v-on:click="showDetails(vm)">
                            <i class="fa fa-info-circle"></i>
                        </button>
                    </td>
                </tr>
            </tbody>
        </table>

        <mm-modal
          v-if="selectedVm != null && !disableDetails"
          :title="selectedVm.name"
          v-on:closed="hideDetails()">
              <vm-details :vm="selectedVm"></vm-details>
        </mm-modal>
    </div>
    `;

export var VmTable = {
    template: template,

    components: {
        MmModal,
        VmDetails,
    },

    filters: {
        formatVMField,
    },

    data() {
        return {
            hidden: () => [],
            sortKey: this.initialKey,
            reverse: this.initialReverse,
            selectedVm: null,
        };
    },

    methods: {
        press(fieldName) {
            if (_.contains(this.hidden, fieldName)) {
                console.log("Unhiding", fieldName);
                this.hidden = _.difference(this.hidden, [fieldName]);
            } else {
                console.log("Hiding", fieldName);
                this.hidden = _.union(this.hidden, [fieldName]);
            }
        },

        isVisible(fieldName) {
            return !_.contains(this.hidden, fieldName);
        },

        toggleSort(fieldName) {
            if (this.sortKey === fieldName) {
                this.reverse = !this.reverse;
            } else {
                this.sortKey = fieldName;
            }
        },

        showDetails(vm) {
            this.selectedVm = vm;
        },

        hideDetails() {
            this.selectedVm = null;
        },
    },

    props: {
        fields: {
            type: Array,
            default: () => ["host", "name", "state", "uptime", "type",
                            "vcpus" ,"memory", "disk", "vlan", "ip",
                            "ip6", "tap", "tags", "cc_active", "vnc_port"]
        },

        initialKey: {
            type: String,
            default: "name"
        },

        initialReverse: {
            type: Boolean,
            default: false
        },

        title: {
            type: String
        },

        vmList: {
            type: Array
        },

        disableDetails: {
            type: Boolean
        },
    },

    computed: {
        vms() {
            // Wrap values because underscore is so dandy
            let vms = _(this.vmList);

            // Sort vms by the key
            let sorted = vms.sortBy((vm) => vm[this.sortKey]);

            return this.reverse ? sorted.reverse() : sorted;
        },

        visible() {
            return _.difference(this.fields, this.hidden);
        }
    }
};
