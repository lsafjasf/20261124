package main

// axml is the ADM metadata of the minimal fixture: one bed object
// (DirectSpeakers) and one moving object (Objects), each on its own track.
const axml = `<?xml version="1.0" encoding="UTF-8"?>
<ebuCoreMain xmlns="urn:ebu:metadata-schema:ebuCore_2014" xmlns:dc="http://purl.org/dc/elements/1.1/">
  <coreMetadata>
    <format>
      <audioFormatExtended version="ITU-R_BS.2076-2">
        <audioProgramme audioProgrammeID="APR_1001" audioProgrammeName="MinProg" start="00:00:00.00000" end="00:00:01.00000">
          <audioContentIDRef>ACO_1001</audioContentIDRef>
        </audioProgramme>
        <audioContent audioContentID="ACO_1001" audioContentName="MinContent">
          <audioObjectIDRef>AO_1001</audioObjectIDRef>
          <audioObjectIDRef>AO_1002</audioObjectIDRef>
        </audioContent>
        <audioObject audioObjectID="AO_1001" audioObjectName="Bed" start="00:00:00.00000" duration="00:00:01.00000">
          <audioPackFormatIDRef>AP_00010002</audioPackFormatIDRef>
          <audioTrackUIDRef>ATU_00000001</audioTrackUIDRef>
        </audioObject>
        <audioObject audioObjectID="AO_1002" audioObjectName="Mover" start="00:00:00.00000" duration="00:00:01.00000">
          <audioPackFormatIDRef>AP_00030001</audioPackFormatIDRef>
          <audioTrackUIDRef>ATU_00000002</audioTrackUIDRef>
        </audioObject>
        <audioPackFormat audioPackFormatID="AP_00010002" audioPackFormatName="BedPack" typeLabel="0001" typeDefinition="DirectSpeakers">
          <audioChannelFormatIDRef>AC_00010001</audioChannelFormatIDRef>
        </audioPackFormat>
        <audioPackFormat audioPackFormatID="AP_00030001" audioPackFormatName="ObjPack" typeLabel="0003" typeDefinition="Objects">
          <audioChannelFormatIDRef>AC_00030001</audioChannelFormatIDRef>
        </audioPackFormat>
        <audioChannelFormat audioChannelFormatID="AC_00010001" audioChannelFormatName="BedL" typeLabel="0001" typeDefinition="DirectSpeakers">
          <audioBlockFormat audioBlockFormatID="AB_00010001_00000001" rtime="00:00:00.00000" duration="00:00:01.00000">
            <speakerLabel>M+030</speakerLabel>
            <position coordinate="azimuth">30.0</position>
            <position coordinate="elevation">0.0</position>
            <position coordinate="distance">1.0</position>
          </audioBlockFormat>
        </audioChannelFormat>
        <audioChannelFormat audioChannelFormatID="AC_00030001" audioChannelFormatName="Mover" typeLabel="0003" typeDefinition="Objects">
          <audioBlockFormat audioBlockFormatID="AB_00030001_00000001" rtime="00:00:00.00000" duration="00:00:00.50000">
            <position coordinate="azimuth">-30.0</position>
            <position coordinate="elevation">0.0</position>
            <position coordinate="distance">1.0</position>
          </audioBlockFormat>
          <audioBlockFormat audioBlockFormatID="AB_00030001_00000002" rtime="00:00:00.50000" duration="00:00:00.50000">
            <position coordinate="azimuth">30.0</position>
            <position coordinate="elevation">0.0</position>
            <position coordinate="distance">1.0</position>
          </audioBlockFormat>
        </audioChannelFormat>
        <audioStreamFormat audioStreamFormatID="AS_00010001" audioStreamFormatName="PCM_BedL" formatLabel="0001" formatDefinition="PCM">
          <audioChannelFormatIDRef>AC_00010001</audioChannelFormatIDRef>
          <audioTrackFormatIDRef>AT_00010001_01</audioTrackFormatIDRef>
        </audioStreamFormat>
        <audioStreamFormat audioStreamFormatID="AS_00030001" audioStreamFormatName="PCM_Mover" formatLabel="0001" formatDefinition="PCM">
          <audioChannelFormatIDRef>AC_00030001</audioChannelFormatIDRef>
          <audioTrackFormatIDRef>AT_00030001_01</audioTrackFormatIDRef>
        </audioStreamFormat>
        <audioTrackFormat audioTrackFormatID="AT_00010001_01" audioTrackFormatName="PCM_BedL" formatLabel="0001" formatDefinition="PCM">
          <audioStreamFormatIDRef>AS_00010001</audioStreamFormatIDRef>
        </audioTrackFormat>
        <audioTrackFormat audioTrackFormatID="AT_00030001_01" audioTrackFormatName="PCM_Mover" formatLabel="0001" formatDefinition="PCM">
          <audioStreamFormatIDRef>AS_00030001</audioStreamFormatIDRef>
        </audioTrackFormat>
        <audioTrackUID UID="ATU_00000001" sampleRate="48000" bitDepth="16">
          <audioTrackFormatIDRef>AT_00010001_01</audioTrackFormatIDRef>
          <audioPackFormatIDRef>AP_00010002</audioPackFormatIDRef>
        </audioTrackUID>
        <audioTrackUID UID="ATU_00000002" sampleRate="48000" bitDepth="16">
          <audioTrackFormatIDRef>AT_00030001_01</audioTrackFormatIDRef>
          <audioPackFormatIDRef>AP_00030001</audioPackFormatIDRef>
        </audioTrackUID>
      </audioFormatExtended>
    </format>
  </coreMetadata>
</ebuCoreMain>
`
