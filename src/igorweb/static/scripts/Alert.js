(function() {
  const template = `
    <div v-if="message != ignored" class="alert alert-warning alert-dismissable">
      <button type="button" class="close" v-on:click="ignore()">&times;</button>

      {{ message }}
    </div>
  `;

  window.Alert = {
    template: template,

    data() {
      return {
        ignored: "",
      };
    },

    props: {
      message: {
        type: String,
      },
    },

    methods: {
      ignore() {
        this.ignored = this.message;
      },
    },

    watch: {
      message(oldMsg, newMsg) {
        // Whenever the message changes, clear out the "ignored" field
        ignored = "";

        // Scroll to the top of the page
        $(window).scrollTop(0);
      },
    },
  };
})();
