package amzconst

// QueryDeviceData is the GraphQL operation used to enrich the basic device
// list with manufacturer/model/endpoint metadata. Originally ported from
// aioamazondevices const/queries.py QUERY_DEVICE_DATA, which split the
// fetch into two displayCategory-filtered sub-queries
// (ALEXA_VOICE_ENABLED + AIR_QUALITY_MONITOR). We now request all
// endpoints in a single call so virtual app-emulation devices (e.g.
// `AioAmazonDevices` registrations of type A2IVLV5VM2W81) are also
// surfaced. The response parser branches on displayCategories to decide
// how to handle each endpoint.
const QueryDeviceData = `
query getDevicesBaseData {
  allDevices: listEndpoints(
    listEndpointsInput: {
      includeHouseholdDevices: true
    }
  ) {
    ...DeviceEndpoints
  }
}

fragment DeviceEndpoints on ListEndpointsResponse {
  endpoints {
    endpointId: id
    friendlyNameObject { value { text } }
    manufacturer { value { text } }
    model { value { text } }
    serialNumber { value { text } }
    softwareVersion { value { text } }
    creationTime
    enablement
    displayCategories {
      all { value }
      primary { value }
    }
    alexaEnabledMetadata {
      iconId
      isVisible
      category
      capabilities
    }
    legacyIdentifiers {
      dmsIdentifier { deviceType { value { text } } }
      chrsIdentifier { entityId }
    }
    legacyAppliance { applianceId }
  }
}
`
