import {formatVMField} from '../filters/formatVMField.js';

const template = `
    <table class="table table-striped table-bordered">
        <thead>
            <tr>
                <th>Field</th>
                <th>Value</th>
            </tr>
        </thead>
        <tbody>
            <tr v-for="field in fields">
                <td>{{ field | formatVMField }}</td>
                <td>{{ vm[field] }}</td>
            </tr>
        </tbody>
    </table>
    `;

export var VmDetails = {
    template: template,

    filters: {
        formatVMField
    },

    props: {
        vm: {
            type: Object
        }
    },

    computed: {
        fields() {
            let keys = _.keys(this.vm);
            return _.sortBy(keys, _.identity);
        }
    }
};
