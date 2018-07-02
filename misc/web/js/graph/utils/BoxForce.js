export function box(width, height) {
    var nodes;
    var maxX = width;
    var maxY = height;
    const buf = 10;

    function force() {
        for (let n of nodes) {
            if (n.x > maxX - buf) {
                n.x = maxX - buf;
            }
            if (n.x < buf) {
                n.x = buf;
            }

            if (n.y > maxY - buf) {
                n.y = maxY - buf;
            }
            if (n.y < buf) {
                n.y = buf;
            }
        }
    }

    force.initialize = function(_) {
        nodes = _;
    };

    force.maxX = function(_) {
        return arguments.length ? (maxX = +_, force) : maxX;
    };

    force.maxY = function(_) {
        return arguments.length ? (maxY = +_, force) : maxY;
    };

    return force;
}
