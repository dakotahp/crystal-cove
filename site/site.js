(function (root) {
  const CrystalCove = {};

  if (typeof module === 'object' && module.exports) module.exports = CrystalCove;
  else root.CrystalCove = CrystalCove;
})(typeof window === 'undefined' ? globalThis : window);
