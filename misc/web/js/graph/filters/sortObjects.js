"use strict";

Vue.filter('sortObjects', function (value, sortBy) {
  if (!_.isArray(value)) {
    console.log("Tried to sort non-Array");
    return value;
  }

  if (!_.every(value, (v) => _.isObject(v))) {
    console.log("Tried to sort Array containing non-object");
    return value;
  }

  return _.sortBy(value, (v) => v[sortBy]);
});
