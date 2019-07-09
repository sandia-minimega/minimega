'use strict';

(function() {
  const template = ''
    + '<div class="alert alert-warning" v-if="message != ignored">'
    + '  <button class="close" type="button" v-on:click="ignore()">&times;</button>'
    + '  {{ message }}'
    + '</div>';
  window.Alert = {
    template: template,
    data: function data() {
      return {
        ignored: '',
      };
    },
    props: {
      message: {
        type: String,
      },
    },
    methods: {
      ignore: function ignore() {
        this.ignored = this.message;
      },
    },
    watch: {
      message: function message(oldMsg, newMsg) {
        // Whenever the message changes, clear out the "ignored" field
        ignored = ''; // Scroll to the top of the page

        $(window).scrollTop(0);
      },
    },
  };
})();
