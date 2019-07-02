'use strict';

(function() {
  const template = ''
    + '<div'
    + '  style="margin-bottom: 20px; padding-bottom: 20px; border-bottom: 1px solid #e9ecef;"'
    + '>'
    + '  <alert :message="serverMessage"></alert>'
    + '  <table class="table table-borderless">'
    + '    <thead>'
    + '      <tr>'
    + '        <th scope="col">Start Time</th>'
    + '        <th scope="col">End Time</th>'
    + '        <th scope="col"></th>'
    + '      </tr>'
    + '    </thead>'
    + '    <tbody>'
    + '      <template v-if="speculations.length == 0">'
    + '        <tr>'
    + '          <td>'
    + '            <i>One moment...</i>'
    + '          </td>'
    + '        </tr>'
    + '      </template>'
    + ''
    + '      <template v-for="spec in speculations">'
    + '        <tr>'
    + '          <td class="align-middle">{{ spec.Start }}</td>'
    + '          <td class="align-middle">{{ spec.End }}</td>'
    + '          <td>'
    + '            <button'
    + '              class="modalbtn igorbtn btn btn-primary modalcommand reserve"'
    + '              style="background-color: #a975d6; border-color: #a975d6; margin-left: 38px;"'
    + '              type="button"'
    + '              v-on:click="$emit(\'reserve\', spec.Formatted)"'
    + '            >'
    + '              <span>Use Window</span>'
    + '            </button>'
    + '          </td>'
    + '        </tr>'
    + '      </template>'
    + '    </tbody>'
    + '  </table>'
    + '</div>'
    + '';
  window.SpeculateTable = {
    template: template,
    components: {
      Alert: Alert,
    },
    props: {
      cmd: {
        type: String,
      },
    },
    data: function data() {
      return {
        speculations: [],
        serverMessage: '',
      };
    },
    mounted: function mounted() {
      const _this = this;

      $.get('run/', {
        run: ''.concat(this.cmd, ' -s'),
      }, function(data) {
        const response = JSON.parse(data);
        _this.speculations = response.Extra;
        const msg = response.Message;

        if (msg.match(/^AVAILABLE/)) {
          _this.serverMessage = 'Speculation successful';
        } else {
          _this.serverMessage = response.Message;
        }
      });
    },
  };
})();
