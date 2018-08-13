// box is a force designed to be used with a d3 force simulation. It
// defines a rectangular barrier that nodes cannot pass.
//
// It's useful for ensuring that nodes remain in sight.
//
// For additional reference, refer to other force implementations
// found in https://github.com/d3/d3-force
export function box(width, height) {
  let nodes;
  let maxX = width;
  let maxY = height;
  const buf = 10;

  // Applies the force to the nodes
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

  // Initializes the force
  force.initialize = function(_) {
    nodes = _;
  };

  // Sets the maximum X coordinate that nodes are allowed to visit
  force.maxX = function(_) {
    return arguments.length ? (maxX = +_, force) : maxX;
  };

  // Sets the maximum Y coordinate that nodes are allowed to visit
  force.maxY = function(_) {
    return arguments.length ? (maxY = +_, force) : maxY;
  };

  return force;
}
