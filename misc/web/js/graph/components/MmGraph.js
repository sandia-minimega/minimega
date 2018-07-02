import {box} from '../utils/BoxForce.js';

export var MmGraph = {
    inject: ['provider'],

    data () {
        return {
            simulation: d3.forceSimulation()
                .force("link", d3.forceLink().id(function(d) { return d.id; }))
                .force("charge", d3.forceManyBody())
                .force("center", d3.forceCenter())
                .force("box", box(0, 0))
                .on("end", () => console.log("Simulation cooled"))
        };
    },

    props: {
        nodes: {
            type: Array,
        },

        nodeStyle: {
            type: Object,
        },

        links: {
            type: Array,
        },
    },

    computed: {
        adjustedNodes() {
            const nodeIDs = this.nodes.map( (n) => n.id );

            return _.chain([this.simulation.nodes(), this.nodes])
                .flatten()
                .groupBy( (n) => n.id )
                .filter( (n, id) => _.contains(nodeIDs, id) )
                .map( (nodes) => _.extend(...nodes) )
                .each( (n) => n.vx = n.vy = 0 )
                .value();
        }
    },

    methods: {
        width() {
            return this.provider.context.canvas.width;
        },

        height() {
            return this.provider.context.canvas.height;
        },

        adjustCenter() {
            this.simulation.force("center")
                .x(this.width() / 2)
                .y(this.height() / 2);

            this.simulation.force("box")
                .maxX(this.width())
                .maxY(this.height());
        },

        recenter() {
            this.simulation.stop();

            this.adjustCenter();

            const center = [this.width()/2, this.height()/2];
            this.simulation.nodes().forEach((n) => {
                [n.x, n.y] = center;
            });

            this.simulation
                .alpha(1)
                .alphaDecay(0.01)
                .alphaTarget(0)
                .restart();
        },
    },

    beforeDestroy() {
        window.removeEventListener('resize', this.handleResize);
    },

    mounted () {
        this.decayTimer = null;

        this.handleResize = () => {
            console.log("Resize");
            let canvas = this.provider.context.canvas;
            canvas.width = canvas.parentElement.clientWidth;
            canvas.height = $(window).height()*0.8;

            this.adjustCenter();

            this.simulation.restart();
        }
        window.addEventListener('resize', this.handleResize);
    },

    render() {
        if(!this.provider.context) {
            return;
        }

        const context = this.provider.context;

        let simulation = this.simulation;
        let graph = this.graph;

        this.handleResize();
        setupDragging(context, simulation);

        d3.select(context.canvas)
            .on("click", () => {
                    const [x, y] = d3.mouse(context.canvas);
                    const subject = simulation.find(x, y, 10);

                    this.$emit('node-click', subject ? subject.id : null);
            });

        simulation
            .on("tick", () => {
                context.clearRect(0, 0, this.width(), this.height());

                context.beginPath();
                this.links.forEach(drawLink.bind(this));
                context.strokeStyle = "#aaa";
                context.stroke();

                simulation.nodes().forEach(drawNode.bind(this));

            });

        function drawLink (link) {
            context.moveTo(link.source.x, link.source.y);
            context.lineTo(link.target.x, link.target.y);
        }

        function drawNode(node)  {
            const style = _.defaults(
                this.nodeStyle[node.id],
                {
                    radius: 5,
                    fill: "#FF0000"
                });


            context.beginPath();
            context.fillStyle = style.fill;
            context.moveTo(node.x + style.radius, node.y);
            context.arc(node.x, node.y, style.radius, 0, 2 * Math.PI);
            context.fill();
        }

        simulation
            .nodes(this.adjustedNodes);

        simulation.force("link")
            .links(this.links);

        simulation
            .alphaDecay(0.01)
            .alphaTarget(0.3)
            .restart();

        if (this.decayTimer) {
            clearTimeout(this.decayTimer);
        }

        this.decayTimer = setTimeout(() => {
            simulation.alphaTarget(0);
        }, 3000);

    }
};

function setupDragging(context, simulation) {
    d3.select(context.canvas)
        .call(d3.drag()
              .container(context.canvas)
              .subject(dragsubject)
              .on("start", dragstarted)
              .on("drag", dragged)
              .on("end", dragended));

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
