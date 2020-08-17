<template>
  <div class="content">
    <template v-if="disabled">
      <section class="hero is-light is-bold is-large">
        <div class="hero-body">
          <div class="container" style="text-align: center">
            <h1 class="title">
              Nothing to see here... logs have been disabled server-side.
            </h1>
          </div>
        </div>
      </section>
    </template>
    <template v-else-if="logs.length == 0">
      <section class="hero is-light is-bold is-large">
        <div class="hero-body">
          <div class="container" style="text-align: center">
            <h1 class="title">
              There are no logs!
            </h1>
          </div>
        </div>
      </section>
    </template>
    <template v-else>
      <hr>
      <b-field position="is-right">
        <b-autocomplete v-model="searchLog"
                        placeholder="Search a log"
                        icon="search"
                        :data="filteredData"
                        @select="option => filtered = option">
          <template slot="empty">
            No results found
          </template>
        </b-autocomplete>
        <p class='control'>
          <button class='button' style="color:#686868" @click="searchLog = ''">
            <b-icon icon="window-close"></b-icon>
          </button>
        </p>
      </b-field>
      <div>
        <b-table
          :data="filteredLogs"
          :paginated="table.isPaginated && paginationNeeded"
          :per-page="table.perPage"
          :current-page.sync="table.currentPage"
          :pagination-simple="table.isPaginationSimple"
          :pagination-size="table.paginationSize"
          :default-sort-direction="table.defaultSortDirection"
          default-sort="timestamp">
          <template slot-scope="props">
            <b-table-column field="timestamp" label="Timestamp" width="200" sortable>
              {{ props.row.timestamp }}
            </b-table-column>
            <b-table-column field="source" label="Source" sortable centered>
              {{ props.row.source }}
            </b-table-column>            
            <b-table-column field="level" label="Level" centered>
              <span class="tag" :class="decorator( props.row.level )">
                {{ props.row.level }}
              </span>
            </b-table-column>
            <b-table-column field="log" label="Log">
              {{ props.row.log }}
            </b-table-column>
          </template>
        </b-table>
        <b-field v-if="paginationNeeded" grouped position="is-right">
          <div class="control is-flex">
            <b-switch v-model="table.isPaginated" size="is-small" type="is-light">Pagenate</b-switch>
          </div>
        </b-field>
      </div>
    </template>
  </div>
</template>

<script>
  export default {
    beforeDestroy () {
      this.$options.sockets.onmessage = null;
      this.logs = [];
    },

    async created () {
      try {
        let resp = await this.$http.get('logs');
        let state = await resp.json();

        // Note that sometimes this function gets called more
        // than once, and sometimes `state` ends up being
        // null, perhaps due to multipart responses?
        if ( state ) {
          this.logs.push(...state.logs);
        }

        this.$options.sockets.onmessage = this.handler;
      } catch (resp) {
        if (resp.status == 501) {
          this.disabled = true
        } else {
          this.$buefy.toast.open({
            message: 'Getting the past hour of logs failed with ' + resp.status + ' status.',
            type: 'is-danger',
            duration: 4000
          });
        }
      }
    },
    
    computed: {
      filteredLogs: function() {
        let logs = this.logs;
        let filters = { 'sources': [], 'levels': [] };

        let tokens = this.searchLog.split(' ');

        for ( let i = tokens.length - 1; i >= 0; i-- ) {
          let token = tokens[i];

          if ( token.includes(':') ) {
            let filter = token.split(':');

            switch ( filter[0].toLowerCase() ) {
              case 'source': {
                filters['sources'] = filters['sources'].concat(filter[1].split(',').map( f => f.toLowerCase() ));

                break;
              }

              case 'level': {
                filters['levels'] = filters['levels'].concat(filter[1].split(',').map( f => f.toLowerCase() ));

                break;
              }
            }

            tokens.splice(i, 1);
          }
        }
        
        let log_re = new RegExp( tokens.join(' '), 'i' );
        let data = [];
        
        for ( let i in logs ) {
          let log = logs[i];

          if ( filters['sources'].length == 0 || filters['sources'].includes(log.source.toLowerCase()) ) {
            if ( filters['levels'].length == 0 || filters['levels'].includes(log.level.toLowerCase()) ) {
              if ( log.log.match( log_re ) ) {
                data.push( log );
              }
            }
          }
        }

        return data;
      },
      
      filteredData () {
        let logs = this.logs.map( log => { return log.log; } );

        return logs.filter(
          log => log.toString().toLowerCase().indexOf( this.searchLog.toLowerCase() ) >= 0
        )
      },
      
      paginationNeeded () {
        if ( this.logs.length <= 10 ) {
          return false;
        } else {
          return true;
        }
      }
    },

    methods: {
      handler ( event ) {
        console.log(this.logs.length);
        event.data.split(/\r?\n/).forEach( m => {
          let msg = JSON.parse( m );
          this.handle( msg );
        });
      },
    
      handle ( msg ) {      
        if ( msg.resource.type == 'log' ) {
//           this.$store.commit( 'LOG', { "log": msg.result } );
          this.logs.push( msg.result );
        }
      },
    
      decorator ( severity ) {
      // severity low -> high
      // debug, info, warn, error, fatal
      
        if ( severity == "ERROR" || severity == "FATAL" ) {
          return 'is-danger';
        } else if ( severity == "WARN" ) {
          return 'is-warning';
        } else if ( severity == "INFO" ) {
          return 'is-info';
        } else {
          return 'is-primary';
        }
      }
    },

    data () {
      return {
        table: {
          striped: true,
          isPaginated: true,
          isPaginationSimple: true,
          paginationSize: 'is-small',
          defaultSortDirection: 'desc',
          currentPage: 1,
          perPage: 10
        },
        disabled: false,
        logs: [],
        searchLog: ''
      }
    }
  }
</script>

<style scoped>
  div.autocomplete >>> a.dropdown-item {
    color: #383838 !important;
  }
</style>
