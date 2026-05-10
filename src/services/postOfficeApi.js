import { isMockMode } from "../lib/config/runtime";
import { livePostOfficeAdapter } from "./adapters/livePostOfficeAdapter";
import { mockPostOfficeAdapter } from "./adapters/mockPostOfficeAdapter";

export const postOfficeApi = isMockMode ? mockPostOfficeAdapter : livePostOfficeAdapter;
