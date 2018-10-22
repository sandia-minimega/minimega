/* global d3, _ */

(function() {
  // MmGraph is a Vue component that contains a canvas-based graph
  // backed by https://github.com/d3/d3-force
  //
  // MmGraph must be used in conjunction with MmCanvas.
  //
  // Component Properties:
  //
  //    - nodes: An Array of Objects specifying node information. See
  //             below for details about expected properties
  //
  //    - links: An Array of Objects specifying link information. See
  //             below for details about expected properties
  //
  // Node Object Properties:
  //
  //   Each passed node Object can/should have the following properties
  //
  //     - id: A unique identifier for the node
  //     - radius: The radius of the drawn node (defaults to 5)
  //     - fillStyle: The fill style for the drawn node (defaults to red)
  //
  // Link Object Properties:
  //
  //   Each passed link Object can/should have the following properties
  //
  //     - source: The id of the link's source node
  //     - target: The id of the link's target node
  //     - stroke: The stroke style for the drawn link (defaults to black)
  //
  // Events:
  //
  //   - node-click: Fires whenever the graph is clicked. If the click
  //                 occurs close to a node, the node's "id" is passed
  //                 with the event. Otherwise, it's null.
  //
  // Example:
  //
  //   For a three-node/two-link graph, try the following:
  //
  //   <mm-canvas>
  //     <mm-graph
  //      :nodes="[{id:'1'}, {id:'2'}, {id:'3'}]"
  //      :links="[{source:'1', target:'2'}, {source:'1', target:'3'}]"
  //      v-on:node-click="doSomething($event)">
  //     </mm-graph>
  //   </mm-canvas>
  //


  // Default style for a node
  const DEFAULT_NODE_STYLE = {
    radius: 5,
    fillStyle: 'red',
  };

  // Default style for a link
  const DEFAULT_LINK_STYLE = {
    strokeStyle: 'black',
  };

  window.MmGraph = {
    // Inject behavior from MmCanvas
    inject: ['provider'],

    // Local data for MmGraph
    data() {
      return {
        // MmGraph's copy of the nodes. **DO NOT** let d3 modify.
        simNodes: [],

        // MmGraph's copy of the links. **DO NOT** let d3 modify.
        simLinks: [],

        // The d3 force simulation -- All initialized with forces:
        //   - link: Force between linked nodes
        //   - charge: Force between unlinked nodes
        //   - radial: Force drawing nodes to the circumference of
        //             a circle centered at the center of the canvas
        //   - box: Force keeping nodes within the bounds of the canvas
        simulation: d3.forceSimulation()
          .force('link', d3.forceLink().id(function(d) {
            return d.id;
          }))
          .force('charge', d3.forceManyBody())
          .force('radial', d3.forceRadial(0))
          .force('box', box(0, 0))
          .on('end', () => console.log('Simulation cooled')), // log when cool
      };
    },

    // Component properties. This is the data that is passed to the
    // mm-graph tag.
    //
    // These values should be treated as read-only. Data is passed
    // from the parent component to MmGraph. All changes to these
    // values will be obliterated whenever the parent updates.
    props: {
      // An array of Objects specifying node information
      nodes: {
        type: Array,
        validator: (value) => {
          // Every node in the Array needs an "id" property
          return _.every(value, (n) => _.has(n, 'id'));
        },
      },

      // An array of Objects specifying link information
      links: {
        type: Array,
        validator: (value) => {
          // Every node in the Array needs "source" and "target" properties
          return _.every(value, (n) => {
            return _.has(n, 'source') && _.has(n, 'target');
          });
        },
      },
    },

    // Watch for changes to values.
    //
    // Vue will react to changes in address, which, for our graph,
    // will result in a bunch of unnecessary rendering and twitching.
    // here, we set up watchers, so that we only react to changes in
    // **VALUE**.
    watch: {
      // Watch for changes to "nodes" prop
      nodes(newNodes, oldNodes) {
        // Do a deep comparison for equality
        if (!_.isEqual(newNodes, oldNodes)) {
          // Hang on new node data if the value has changed
          console.log('Nodes updated');
          this.simNodes = newNodes;
        }
      },

      // Watch for changes to "links" prop
      links(newLinks, oldLinks) {
        // Do a deep comparison for equality
        if (!_.isEqual(newLinks, oldLinks)) {
          // Hang on new link data if the value has changed
          console.log('Links updated');
          this.simLinks = newLinks;
        }
      },
    },

    // Convenience methods!
    methods: {

      // Returns the width of the canvas
      width() {
        return this.provider.context.canvas.width;
      },

      // Returns the height of the canvas
      height() {
        return this.provider.context.canvas.height;
      },

      // Returns an Array ([x, y]) containing the coordinates of the
      // center of the canvas
      center() {
        return [this.width()/2, this.height()/2];
      },

      // Updates simulation forces to match the dimensions of the
      // camvas. Note that this does not restart the simulation.
      adjustCenter() {
        // Center the radial force around the center of the canvas
        const [x, y] = this.center();
        this.simulation.force('radial')
          .x(x)
          .y(y);

        // Set the bounds of the box force to the perimeter of the canvas
        this.simulation.force('box')
          .maxX(this.width())
          .maxY(this.height());
      },

      // Return a **COPY** of the Array of node objects. The return
      // value is not cached. It's safe to pass this to d3.
      adjustedNodes() {
        // IDs of the nodes we want to keep
        const nodeIDs = this.simNodes.map( (n) => n.id );

        return _.chain([this.simulation.nodes(), this.simNodes])
        // Combine all node objects from d3 and MmGraph
          .flatten()
        // Group node objects by "id"
          .groupBy( (n) => n.id )
        // Filter out old nodes
          .filter( (n, id) => _.contains(nodeIDs, id) )
        // Combine all node objects (from d3 and MmGraph), creating the copy
          .map( (nodes) => _.extend({}, ...nodes) )
        // Zero out velocities
          .each( (n) => n.vx = n.vy = 0 )
        // Return the final result
          .value();
      },

      // Return a **COPY** of the Array of link objects. The return
      // value is not cached. It's safe to pass this to d3.
      adjustedLinks() {
        return _.chain(this.simLinks)
          .map( (link) => _.extend({}, link) ) // shallow copy
          .value();
      },

      // Adjusts the simulation forces to dimensions/center of the
      // canvas, scoots all nodes to the center of the page, and
      // restarts (reheats) the simulation.
      recenter() {
        this.simulation.stop();

        this.adjustCenter();

        this.simulation.nodes().forEach((n) => {
          [n.x, n.y] = this.center();
        });

        this.simulation
          .alpha(1)
          .restart();
      },
    },

    // Runs after the Vue component has been mounted and is
    // ready-to-go
    mounted() {
      // Hang on to the node/link info passed through props
      this.simNodes = this.nodes;
      this.simLinks = this.links;

      // Setup a handler to restart stuff whenever the page is resized
      this.handleResize = () => {
        console.log('Resize');
        let canvas = this.provider.context.canvas;
        canvas.width = canvas.parentElement.clientWidth;
        canvas.height = $(window).height()*0.75;

        this.adjustCenter();

        this.simulation
          .restart();
      };
      window.addEventListener('resize', this.handleResize);
    },

    // Runs right before the Vue component is cleaned up
    beforeDestroy() {
      // Remove our window resize listener
      window.removeEventListener('resize', this.handleResize);
    },

    // Redraws the MmGraph whenever relevant data changes
    //
    // For example, when the new nodes are added to the "nodes" prop
    // (passed by the parent component), we'll need to add it to the
    // graph and redraw everything.
    render() {
      console.log('Render');

      // If the MmCanvas hasn't been fully setup, we'll need to wait.
      if (!this.provider.context) {
        console.log('Context not ready');
        return;
      }

      const context = this.provider.context;
      let simulation = this.simulation;
      let canvas = this.provider.context.canvas;

      // Adjust the canvas dimensions, based on the dimensions of
      // its parent element.
      canvas.width = canvas.parentElement.clientWidth;
      canvas.height = $(window).height()*0.75;

      // Whenever a user clicks (near) a node, emit a "node-click"
      // event along with the "id" of the clicked node.
      d3.select(context.canvas)
        .on('click', () => {
          const [x, y] = d3.mouse(context.canvas);
          const subject = simulation.find(x, y, 10);

          this.$emit('node-click', subject ? subject.id : null);
        });

      // Repaint the nodes and links on the canvas whenever the
      // simulation ticks
      simulation
        .on('tick', () => {
          context.clearRect(0, 0, this.width(), this.height());
          simulation.force('link').links().forEach(drawLink.bind(this));
          simulation.nodes().forEach(drawNode.bind(this));
        });

      // Draw a link
      function drawLink(link) {
        const l = _.defaults(link, DEFAULT_LINK_STYLE);

        context.beginPath();
        context.strokeStyle = l.strokeStyle;

        context.moveTo(l.source.x, l.source.y);
        context.lineTo(l.target.x, l.target.y);
        context.stroke();
      }

      // Draw a node
      function drawNode(node) {
        const n = _.defaults(node, DEFAULT_NODE_STYLE);

        context.beginPath();
        context.fillStyle = n.fillStyle;
        context.moveTo(n.x + n.radius, n.y);
        context.arc(n.x, n.y, n.radius, 0, 2 * Math.PI);
        context.fill();
      }

      // Setup dragging/dropping
      setupDragging(context, simulation);

      // Adjust simulation forces to match canvas dimensions
      this.adjustCenter();

      // Update the nodes in the simulation
      simulation
        .nodes(this.adjustedNodes());

      // Update the links in the simulation
      simulation.force('link')
        .links(this.adjustedLinks());

      // Restart (reheat) the simulation
      simulation
        .alpha(1)
        .restart();
    },
  };

  // Sets up dragging/dropping
  function setupDragging(context, simulation) {
    d3.select(context.canvas)
      .call(d3.drag()
        .container(context.canvas)
        .subject(dragsubject)
        .on('start', dragstarted)
        .on('drag', dragged)
        .on('end', dragended));

    function dragsubject() {
      return simulation.find(d3.event.x, d3.event.y, 10);
    }

    function dragstarted() {
      if (!d3.event.active) simulation.alphaTarget(0.3).restart();
      d3.event.subject.fx = d3.event.subject.x;
      d3.event.subject.fy = d3.event.subject.y;
    }

    function dragged() {
      d3.event.subject.fx = d3.event.x;
      d3.event.subject.fy = d3.event.y;
    }

    function dragended() {
      if (!d3.event.active) simulation.alphaTarget(0);
      d3.event.subject.fx = null;
      d3.event.subject.fy = null;
    }
  }
})();
