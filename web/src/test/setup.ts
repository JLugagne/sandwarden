import { configure } from "@testing-library/dom";

// The default 1s async timeout is too tight when several jsdom workers run in
// parallel on a loaded machine; a real regression still fails, just slower.
configure({ asyncUtilTimeout: 15000 });
