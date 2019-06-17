(function() {
  const template = `
    <div class="mdl" style="margin-bottom: 20px; padding-bottom: 20px; border-bottom: 1px solid #e9ecef;">
      <alert :message="serverMessage"></alert>
      <table class="mdl table table-borderless">
        <thead class="mdl">
          <tr class="mdl">
            <th class="mdl" scope="col">Start Time</th>
            <th class="mdl" scope="col">End Time</th>
            <th class="mdl" scope="col"></th>
          </tr>
        </thead>
        <tbody class="mdl" id="spec_table">
          <template v-if="speculations.length == 0">
            <tr class="mdl">
              <td><i>One moment...</i></td>
            </tr>
          </template>

          <template v-for="spec in speculations">
            <tr class="mdl">
              <td class="align-middle mdl">{{ spec.Start }}</td>
              <td class="align-middle mdl">{{ spec.End }}</td>
              <td class="mdl">
                <button
                  type="button"
                  style="background-color: #a975d6; border-color: #a975d6; margin-left: 38px;"
                  class="modalbtn specreserve igorbtn btn btn-primary mdl modalcommand reserve"
                  v-on:click="$emit('reserve', spec.Formatted)">
                    <span class="mdl mdlcmdtext">Use Window</span>
                </button>
              </td>
            </tr>
          </template>
        </tbody>
      </table>
    </div>
  `;

  window.SpeculateTable = {
    template: template,

    components: {
      Alert,
    },

    props: {
      cmd: {
        type: String,
      },
    },

    data() {
      return {
        speculations: [],
        serverMessage: '',
      };
    },

    mounted() {
      $.get(
          'run/',
          {run: `${this.cmd} -s`},
          (data) => {
            const response = JSON.parse(data);
            this.speculations = response.Extra;

            const msg = response.Message;
            if (msg.match(/^AVAILABLE/)) {
              this.serverMessage = 'Speculation successful';
            } else {
              this.serverMessage = response.Message;
            }
          }
      );
    },
  };
})();
