module Types exposing (..)

import Json.Decode as Decode exposing (Decoder)


type alias State =
    { resources : List Resource
    , factories : List Factory
    , sinks : List Sink
    , transports : List Transport
    , tick : Int
    , running : Bool
    , bounds : Bounds
    }


type alias Bounds =
    { xmin : Int
    , xmax : Int
    , ymin : Int
    , ymax : Int
    }


type alias Resource =
    { location : Location
    , recipe : String
    , product : String
    , profitability : Float
    , active : Bool
    }


type alias Factory =
    { location : Location
    , recipe : String
    , products : List String
    , profitability : Float
    }


type alias Sink =
    { location : Location
    , label : String
    }


type alias Transport =
    { origin : Location
    , destination : Location
    , rate : Float
    }


type alias Location =
    { x : Int
    , y : Int
    }


initialState : State
initialState =
    { resources = []
    , factories = []
    , sinks = []
    , transports = []
    , tick = 0
    , running = False
    , bounds =
        { xmin = -100
        , xmax = 100
        , ymin = -100
        , ymax = 100
        }
    }



-- JSON decoders


locationDecoder : Decoder Location
locationDecoder =
    Decode.map2 Location
        (Decode.field "x" Decode.int)
        (Decode.field "y" Decode.int)


transportDecoder : Decoder Transport
transportDecoder =
    Decode.map3 Transport
        (Decode.field "origin" locationDecoder)
        (Decode.field "destination" locationDecoder)
        (Decode.field "rate" Decode.float)


resourceDecoder : Decoder Resource
resourceDecoder =
    Decode.map5 Resource
        (Decode.field "location" locationDecoder)
        (Decode.field "recipe" Decode.string)
        (Decode.field "product" Decode.string)
        (Decode.field "profitability" Decode.float)
        (Decode.field "active" Decode.bool)


factoryDecoder : Decoder Factory
factoryDecoder =
    Decode.map4 Factory
        (Decode.field "location" locationDecoder)
        (Decode.field "recipe" Decode.string)
        (Decode.field "products" (Decode.list Decode.string))
        (Decode.field "profitability" Decode.float)


sinkDecoder : Decoder Sink
sinkDecoder =
    Decode.map2 Sink
        (Decode.field "location" locationDecoder)
        (Decode.field "label" Decode.string)


stateDecoder : Decoder State
stateDecoder =
    Decode.map7 State
        (Decode.field "resources" (Decode.list resourceDecoder))
        (Decode.field "factories" (Decode.list factoryDecoder))
        (Decode.field "sinks" (Decode.list sinkDecoder))
        (Decode.field "transports" (Decode.list transportDecoder))
        (Decode.field "tick" Decode.int)
        (Decode.field "running" Decode.bool)
        (Decode.field "bounds" boundsDecoder)


boundsDecoder : Decoder Bounds
boundsDecoder =
    Decode.map4 Bounds
        (Decode.field "xmin" Decode.int)
        (Decode.field "xmax" Decode.int)
        (Decode.field "ymin" Decode.int)
        (Decode.field "ymax" Decode.int)


type alias Recipe =
    { name : String
    , inputs : List Product
    , outputs : List Product
    , active : Bool
    }


type alias Product =
    { name : String
    , rate : Float
    }


recipeDecoder : Decoder Recipe
recipeDecoder =
    Decode.map4 Recipe
        (Decode.field "name" Decode.string)
        (Decode.field "inputs" (Decode.list productDecoder))
        (Decode.field "outputs" (Decode.list productDecoder))
        (Decode.field "active" Decode.bool)


productDecoder : Decoder Product
productDecoder =
    Decode.map2 Product
        (Decode.field "name" Decode.string)
        (Decode.field "rate" Decode.float)
